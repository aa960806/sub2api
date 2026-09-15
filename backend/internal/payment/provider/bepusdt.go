package provider

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// Bepusdt integrates BEpusdt's JSON transaction API.
type Bepusdt struct {
	instanceID string
	config     map[string]string
	client     *http.Client
}

func NewBepusdt(instanceID string, config map[string]string) (*Bepusdt, error) {
	for _, key := range []string{"apiBase", "token"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("bepusdt config missing required key: %s", key)
		}
	}
	cfg := map[string]string{}
	for k, v := range config {
		cfg[k] = v
	}
	base := strings.TrimRight(strings.TrimSpace(cfg["apiBase"]), "/")
	if u, err := url.Parse(base); err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("bepusdt apiBase must be an absolute URL")
	}
	cfg["apiBase"] = base
	if fiat := strings.TrimSpace(cfg["fiat"]); fiat != "" && !strings.EqualFold(fiat, "CNY") {
		return nil, fmt.Errorf("bepusdt fiat must be CNY to match account billing currency")
	}
	cfg["fiat"] = "CNY"
	cfg["tradeType"] = strings.TrimSpace(cfg["tradeType"])
	cfg["timeout"] = strings.TrimSpace(cfg["timeout"])
	if timeout := strings.TrimSpace(cfg["timeout"]); timeout != "" {
		n, err := strconv.Atoi(timeout)
		if err != nil || n < 120 {
			return nil, fmt.Errorf("bepusdt timeout must be at least 120 seconds")
		}
	}
	return &Bepusdt{instanceID: instanceID, config: cfg, client: &http.Client{Timeout: 20 * time.Second}}, nil
}
func (b *Bepusdt) Name() string        { return "BEpusdt" }
func (b *Bepusdt) ProviderKey() string { return "bepusdt" }
func (b *Bepusdt) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeBepusdt, payment.TypeBepusdtBEP20, payment.TypeBepusdtTRC20, payment.TypeBepusdtERC20, payment.TypeBepusdtTON}
}
func (b *Bepusdt) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"apiBase": b.config["apiBase"]}
}

func (b *Bepusdt) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := bepPositiveAmount(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	if strings.TrimSpace(req.OrderID) == "" {
		return nil, fmt.Errorf("bepusdt create missing order_id")
	}
	// A network-specific visible payment type takes precedence over the
	// instance's legacy tradeType setting. Legacy "bepusdt" requests retain
	// the configured tradeType (or the historical TRC20 default).
	tradeType := b.config["tradeType"]
	switch strings.ToLower(strings.TrimSpace(req.PaymentType)) {
	case payment.TypeBepusdtBEP20:
		tradeType = "usdt.bep20"
	case payment.TypeBepusdtTRC20:
		tradeType = "usdt.trc20"
	case payment.TypeBepusdtERC20:
		tradeType = "usdt.erc20"
	case payment.TypeBepusdtTON:
		tradeType = "usdt.ton"
	}
	if tradeType == "" {
		tradeType = "usdt.trc20"
	}
	fiat := b.config["fiat"]
	if fiat == "" {
		fiat = "CNY"
	}
	notifyURL, returnURL := req.NotifyURL, req.ReturnURL
	if notifyURL == "" {
		notifyURL = b.config["notifyUrl"]
	}
	if returnURL == "" {
		returnURL = b.config["returnUrl"]
	}
	if !bepHTTPURL(notifyURL) || !bepHTTPURL(returnURL) {
		return nil, fmt.Errorf("bepusdt create requires absolute HTTP(S) notify_url and redirect_url")
	}
	payload := map[string]any{"order_id": req.OrderID, "amount": amount, "fiat": fiat, "trade_type": tradeType, "name": req.Subject, "notify_url": notifyURL, "redirect_url": returnURL}
	if timeout, _ := strconv.Atoi(b.config["timeout"]); timeout > 0 {
		payload["timeout"] = timeout
	}
	payload["signature"] = bepSign(payload, b.config["token"])
	body, err := b.postJSON(ctx, "/api/v1/order/create-transaction", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			TradeID    string `json:"trade_id"`
			PaymentURL string `json:"payment_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bepusdt parse response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bepusdt create failed: %s", resp.Message)
	}
	if strings.TrimSpace(resp.Data.TradeID) == "" || !bepHTTPURL(resp.Data.PaymentURL) {
		return nil, fmt.Errorf("bepusdt create response missing trade_id or valid payment_url")
	}
	return &payment.CreatePaymentResponse{TradeNo: resp.Data.TradeID, PayURL: resp.Data.PaymentURL}, nil
}

func (b *Bepusdt) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	if strings.TrimSpace(tradeNo) == "" {
		return nil, fmt.Errorf("bepusdt query missing trade_id")
	}
	body, err := b.postJSON(ctx, "/api/v1/pay/info", map[string]any{"trade_id": tradeNo, "signature": bepSign(map[string]any{"trade_id": tradeNo}, b.config["token"])})
	if err != nil {
		return nil, err
	}
	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			TradeID      string `json:"trade_id"`
			Status       any    `json:"status"`
			Money        any    `json:"money"`
			ActualAmount any    `json:"actual_amount"`
			Fiat         string `json:"fiat"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bepusdt query parse: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bepusdt query failed: %s", resp.Message)
	}
	if resp.Data.TradeID != tradeNo {
		return nil, fmt.Errorf("bepusdt query returned a different or missing trade_id")
	}
	if !strings.EqualFold(strings.TrimSpace(resp.Data.Fiat), "CNY") {
		return nil, fmt.Errorf("bepusdt query currency must be CNY")
	}
	status, err := bepProviderStatus(resp.Data.Status)
	if err != nil {
		return nil, err
	}
	// /pay/info returns the original fiat total as money. actual_amount is
	// the token quantity and must never be used to credit a CNY order.
	amount, err := bepPositiveAmount(resp.Data.Money)
	if err != nil {
		return nil, fmt.Errorf("bepusdt query invalid money: %w", err)
	}
	metadata, err := bepAmountMetadata(resp.Data.ActualAmount, status != payment.ProviderStatusPaid)
	if err != nil {
		return nil, err
	}
	return &payment.QueryOrderResponse{TradeNo: tradeNo, Status: status, Amount: amount, Metadata: metadata}, nil
}
func (b *Bepusdt) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("bepusdt does not support refunds")
}
func (b *Bepusdt) CancelPayment(ctx context.Context, tradeNo string) error {
	if strings.TrimSpace(tradeNo) == "" {
		return fmt.Errorf("bepusdt cancel missing trade_id")
	}
	body, err := b.postJSON(ctx, "/api/v1/order/cancel-transaction", map[string]any{"trade_id": tradeNo, "signature": bepSign(map[string]any{"trade_id": tradeNo}, b.config["token"])})
	if err != nil {
		return err
	}
	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("bepusdt cancel parse: %w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("bepusdt cancel failed: %s", resp.Message)
	}
	return nil
}

func (b *Bepusdt) VerifyNotification(_ context.Context, raw string, _ map[string]string) (*payment.PaymentNotification, error) {
	values := map[string]any{}
	// BEpusdt signs after json.Unmarshal into map[string]any, so JSON
	// numbers must use Go's float64 formatting (including exponents),
	// while quoted decimal strings retain their original precision.
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("bepusdt notify parse: %w", err)
	}
	signature, _ := values["signature"].(string)
	if subtle.ConstantTimeCompare([]byte(bepSign(values, b.config["token"])), []byte(strings.ToLower(signature))) != 1 {
		return nil, fmt.Errorf("invalid signature")
	}
	tradeID, _ := values["trade_id"].(string)
	orderID, _ := values["order_id"].(string)
	if strings.TrimSpace(tradeID) == "" || strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("bepusdt notify missing trade_id or order_id")
	}
	amount, err := bepPositiveAmount(values["amount"])
	if err != nil {
		return nil, fmt.Errorf("bepusdt notify invalid amount: %w", err)
	}
	if fiat, exists := values["fiat"]; exists && !strings.EqualFold(fmt.Sprint(fiat), "CNY") {
		return nil, fmt.Errorf("bepusdt notify currency must be CNY")
	}
	providerStatus, err := bepProviderStatus(values["status"])
	if err != nil {
		return nil, err
	}
	if providerStatus == payment.ProviderStatusPaid {
		providerStatus = payment.ProviderStatusSuccess
	}
	metadata, err := bepAmountMetadata(values["actual_amount"], providerStatus != payment.ProviderStatusSuccess)
	if err != nil {
		return nil, err
	}
	return &payment.PaymentNotification{TradeNo: tradeID, OrderID: orderID, Amount: amount, Status: providerStatus, RawData: raw, Metadata: metadata}, nil
}

func (b *Bepusdt) postJSON(ctx context.Context, path string, payload map[string]any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("bepusdt encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config["apiBase"]+path, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bepusdt request: %w", err)
	}
	defer resp.Body.Close()
	const maxResponseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("bepusdt read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("bepusdt response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bepusdt HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func bepHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

func bepPositiveAmount(value any) (float64, error) {
	amount, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		return 0, fmt.Errorf("amount must be finite and positive")
	}
	return amount, nil
}

func bepAmountMetadata(actual any, allowZero bool) (map[string]string, error) {
	metadata := map[string]string{"currency": "CNY"}
	if actual != nil && actual != "" {
		amount, err := strconv.ParseFloat(fmt.Sprint(actual), 64)
		if err != nil {
			return nil, fmt.Errorf("bepusdt invalid actual_amount: %w", err)
		}
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || (!allowZero && amount == 0) {
			requirement := "strictly positive"
			if allowZero {
				requirement = "finite and non-negative"
			}
			return nil, fmt.Errorf("bepusdt invalid actual_amount: must be %s", requirement)
		}
		metadata["actual_amount"] = strconv.FormatFloat(amount, 'f', -1, 64)
	}
	return metadata, nil
}

func bepProviderStatus(value any) (string, error) {
	status, err := strconv.Atoi(fmt.Sprint(value))
	if err == nil {
		switch status {
		case 1, 5: // Waiting for payment or blockchain confirmation.
			return payment.ProviderStatusPending, nil
		case 2:
			return payment.ProviderStatusPaid, nil
		case 3, 4, 6: // Expired, cancelled, or failed confirmation.
			return payment.ProviderStatusFailed, nil
		}
	}
	return "", fmt.Errorf("bepusdt invalid order status")
}

func bepSign(values map[string]any, token string) string {
	keys := make([]string, 0, len(values))
	for k, v := range values {
		if k == "signature" || v == nil || fmt.Sprint(v) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k + "=" + fmt.Sprint(values[k]))
	}
	b.WriteString(token)
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

var _ payment.Provider = (*Bepusdt)(nil)
var _ payment.CancelableProvider = (*Bepusdt)(nil)
