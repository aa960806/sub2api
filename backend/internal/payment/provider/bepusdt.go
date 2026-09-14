package provider

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	if timeout := strings.TrimSpace(cfg["timeout"]); timeout != "" {
		n, err := strconv.Atoi(timeout)
		if err != nil || n < 120 {
			return nil, fmt.Errorf("bepusdt timeout must be at least 120 seconds")
		}
	}
	return &Bepusdt{instanceID: instanceID, config: cfg, client: &http.Client{Timeout: 20 * time.Second}}, nil
}
func (b *Bepusdt) Name() string                          { return "BEpusdt" }
func (b *Bepusdt) ProviderKey() string                   { return "bepusdt" }
func (b *Bepusdt) SupportedTypes() []payment.PaymentType { return []payment.PaymentType{"bepusdt"} }
func (b *Bepusdt) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"apiBase": b.config["apiBase"]}
}

func (b *Bepusdt) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	tradeType := b.config["tradeType"]
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
			TradeID      string `json:"trade_id"`
			PaymentURL   string `json:"payment_url"`
			Token        string `json:"token"`
			ActualAmount string `json:"actual_amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bepusdt parse response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bepusdt create failed: %s", resp.Message)
	}
	return &payment.CreatePaymentResponse{TradeNo: resp.Data.TradeID, PayURL: resp.Data.PaymentURL}, nil
}

func (b *Bepusdt) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	body, err := b.postJSON(ctx, "/api/v1/pay/info", map[string]any{"trade_id": tradeNo, "signature": bepSign(map[string]any{"trade_id": tradeNo}, b.config["token"])})
	if err != nil {
		return nil, err
	}
	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			Status       int    `json:"status"`
			Amount       string `json:"amount"`
			Money        string `json:"money"`
			ActualAmount string `json:"actual_amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bepusdt query parse: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bepusdt query failed: %s", resp.Message)
	}
	status := payment.ProviderStatusPending
	if resp.Data.Status == 2 {
		status = payment.ProviderStatusPaid
	} else if resp.Data.Status == 3 {
		status = payment.ProviderStatusFailed
	}
	amount, _ := strconv.ParseFloat(resp.Data.Amount, 64)
	if resp.Data.Amount == "" {
		amount, _ = strconv.ParseFloat(resp.Data.Money, 64)
	}
	actual, _ := strconv.ParseFloat(resp.Data.ActualAmount, 64)
	return &payment.QueryOrderResponse{TradeNo: tradeNo, Status: status, Amount: amount, Metadata: map[string]string{"actual_amount": strconv.FormatFloat(actual, 'f', -1, 64)}}, nil
}
func (b *Bepusdt) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("bepusdt does not support refunds")
}
func (b *Bepusdt) CancelPayment(ctx context.Context, tradeNo string) error {
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
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	values := map[string]any{}
	if err := dec.Decode(&values); err != nil {
		return nil, fmt.Errorf("bepusdt notify parse: %w", err)
	}
	signature, _ := values["signature"].(string)
	if !strings.EqualFold(bepSign(values, b.config["token"]), signature) {
		return nil, fmt.Errorf("invalid signature")
	}
	tradeID, _ := values["trade_id"].(string)
	orderID, _ := values["order_id"].(string)
	amount, _ := strconv.ParseFloat(fmt.Sprint(values["amount"]), 64)
	actual, _ := strconv.ParseFloat(fmt.Sprint(values["actual_amount"]), 64)
	numericStatus, _ := strconv.Atoi(fmt.Sprint(values["status"]))
	providerStatus := payment.ProviderStatusFailed
	if numericStatus == 2 {
		providerStatus = payment.ProviderStatusSuccess
	} else if numericStatus == 1 {
		providerStatus = payment.ProviderStatusPending
	}
	return &payment.PaymentNotification{TradeNo: tradeID, OrderID: orderID, Amount: amount, Status: providerStatus, RawData: raw, Metadata: map[string]string{"actual_amount": strconv.FormatFloat(actual, 'f', -1, 64)}}, nil
}

func (b *Bepusdt) postJSON(ctx context.Context, path string, payload map[string]any) ([]byte, error) {
	data, _ := json.Marshal(payload)
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bepusdt HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
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
