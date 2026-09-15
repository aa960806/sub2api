package provider

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestBepSignMatchesDocumentExampleShape(t *testing.T) {
	values := map[string]any{"order_id": "20220201030210321", "amount": 42, "notify_url": "http://example.com/notify", "redirect_url": "http://example.com/redirect"}
	require.Equal(t, "1cd4b52df5587cfb1968b0c0c6e156cd", bepSign(values, "epusdt_password_xasddawqe"))
}

func TestNewBepusdtValidation(t *testing.T) {
	_, err := NewBepusdt("x", map[string]string{"apiBase": "https://pay.example.com", "token": "t", "fiat": "USD"})
	require.Error(t, err)
	_, err = NewBepusdt("x", map[string]string{"apiBase": "https://pay.example.com", "token": "t", "timeout": "60"})
	require.Error(t, err)
}

// Mirrors upstream app/utils.EpusdtSign, independently of the adapter's helper.
// Upstream decodes numbers to float64 before signing, even when the wire uses
// integers, decimal zeroes, or scientific notation.
func bepUpstreamSignature(t *testing.T, wire []byte) string {
	t.Helper()
	var values map[string]any
	require.NoError(t, json.Unmarshal(wire, &values))
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "signature" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var pairs []string
	for _, key := range keys {
		if values[key] != nil && values[key] != "" {
			pairs = append(pairs, key+"="+fmt.Sprintf("%v", values[key]))
		}
	}
	sum := md5.Sum([]byte(strings.Join(pairs, "&") + "test-token"))
	return hex.EncodeToString(sum[:])
}

func bepTestProvider(t *testing.T, handler http.HandlerFunc) *Bepusdt {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	provider, err := NewBepusdt("bep-test", map[string]string{"apiBase": server.URL, "token": "test-token"})
	require.NoError(t, err)
	return provider
}

func bepCreateRequest() payment.CreatePaymentRequest {
	return payment.CreatePaymentRequest{
		OrderID: "order-1", Amount: "28.88", Subject: "Account recharge",
		NotifyURL: "https://shop.example/api/v1/payment/webhook/bepusdt",
		ReturnURL: "https://shop.example/payment/result",
	}
}

func TestBepusdtCreatePaymentMatchesUpstreamProtocol(t *testing.T) {
	provider := bepTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/order/create-transaction", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		require.Equal(t, bepUpstreamSignature(t, body), payload["signature"])
		require.Equal(t, "order-1", payload["order_id"])
		require.Equal(t, 28.88, payload["amount"])
		require.Equal(t, "CNY", payload["fiat"])
		require.Equal(t, "usdt.trc20", payload["trade_type"])
		require.Equal(t, float64(900), payload["timeout"])
		require.Equal(t, bepCreateRequest().NotifyURL, payload["notify_url"])
		require.Equal(t, bepCreateRequest().ReturnURL, payload["redirect_url"])
		_, _ = io.WriteString(w, `{"status_code":200,"message":"success","data":{"trade_id":"trade-1","amount":"28.88","actual_amount":"4.25","payment_url":"https://pay.example/pay/checkout/trade-1"}}`)
	})
	provider.config["timeout"] = "900"
	resp, err := provider.CreatePayment(context.Background(), bepCreateRequest())
	require.NoError(t, err)
	require.Equal(t, "trade-1", resp.TradeNo)
	require.Equal(t, "https://pay.example/pay/checkout/trade-1", resp.PayURL)
}

func TestBepusdtCreatePaymentSelectsNetworkFromPaymentType(t *testing.T) {
	for _, tc := range []struct {
		paymentType string
		tradeType   string
	}{
		{payment.TypeBepusdtBEP20, "usdt.bep20"},
		{payment.TypeBepusdtTRC20, "usdt.trc20"},
		{payment.TypeBepusdtERC20, "usdt.erc20"},
		{payment.TypeBepusdtTON, "usdt.ton"},
	} {
		t.Run(tc.paymentType, func(t *testing.T) {
			provider := bepTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Equal(t, tc.tradeType, payload["trade_type"])
				require.Equal(t, bepUpstreamSignature(t, body), payload["signature"])
				_, _ = io.WriteString(w, `{"status_code":200,"data":{"trade_id":"trade-network","payment_url":"https://pay.example/pay/trade-network"}}`)
			})
			req := bepCreateRequest()
			provider.config["tradeType"] = "usdt.bep20"
			req.PaymentType = tc.paymentType
			_, err := provider.CreatePayment(context.Background(), req)
			require.NoError(t, err)
		})
	}
}

func TestBepusdtLegacyCreateKeepsConfiguredNetwork(t *testing.T) {
	provider := bepTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "usdt.bep20", payload["trade_type"])
		_, _ = io.WriteString(w, `{"status_code":200,"data":{"trade_id":"trade-legacy","payment_url":"https://pay.example/pay/trade-legacy"}}`)
	})
	provider.config["tradeType"] = "usdt.bep20"
	req := bepCreateRequest()
	req.PaymentType = payment.TypeBepusdt
	_, err := provider.CreatePayment(context.Background(), req)
	require.NoError(t, err)

	registry := payment.NewRegistry()
	registry.Register(provider)
	for _, method := range []string{payment.TypeBepusdtERC20, payment.TypeBepusdtTON} {
		registered, err := registry.GetProvider(method)
		require.NoError(t, err)
		require.Same(t, provider, registered)
		require.Equal(t, payment.TypeBepusdt, registry.GetProviderKey(method))
	}
}

func TestBepusdtRejectsInvalidRequestsWithoutContactingUpstream(t *testing.T) {
	provider := bepTestProvider(t, func(http.ResponseWriter, *http.Request) {
		t.Error("invalid payment request reached upstream")
	})
	for _, amount := range []string{"", "0", "-1", "NaN", "+Inf", "-Inf", "1e400", "garbage"} {
		t.Run("amount_"+amount, func(t *testing.T) {
			req := bepCreateRequest()
			req.Amount = amount
			_, err := provider.CreatePayment(context.Background(), req)
			require.ErrorContains(t, err, "amount")
		})
	}
	for _, mutate := range []func(*payment.CreatePaymentRequest){
		func(req *payment.CreatePaymentRequest) { req.OrderID = " " },
		func(req *payment.CreatePaymentRequest) { req.NotifyURL = "" },
		func(req *payment.CreatePaymentRequest) { req.ReturnURL = "javascript:alert(1)" },
	} {
		req := bepCreateRequest()
		mutate(&req)
		_, err := provider.CreatePayment(context.Background(), req)
		require.Error(t, err)
	}
	_, err := provider.QueryOrder(context.Background(), "")
	require.ErrorContains(t, err, "trade_id")
	require.ErrorContains(t, provider.CancelPayment(context.Background(), ""), "trade_id")
}

func TestBepusdtCreateRejectsInvalidResponses(t *testing.T) {
	for _, response := range []string{
		`{"status_code":400,"message":"no available address"}`,
		`{"status_code":200,"data":{"payment_url":"https://pay.example/checkout"}}`,
		`{"status_code":200,"data":{"trade_id":"trade-1"}}`,
		`{"status_code":200,"data":{"trade_id":"trade-1","payment_url":"javascript:alert(1)"}}`,
		`{"status_code":200,"data":{"trade_id":"trade-1","payment_url":"/checkout"}}`,
		`not json`,
	} {
		t.Run(response, func(t *testing.T) {
			provider := bepTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, response)
			})
			_, err := provider.CreatePayment(context.Background(), bepCreateRequest())
			require.Error(t, err)
		})
	}
}

func TestBepusdtQueryUsesFiatMoneyAndMapsStates(t *testing.T) {
	for status, expected := range map[int]string{
		1: payment.ProviderStatusPending, 2: payment.ProviderStatusPaid,
		3: payment.ProviderStatusFailed, 4: payment.ProviderStatusFailed,
		5: payment.ProviderStatusPending, 6: payment.ProviderStatusFailed,
	} {
		for _, money := range []string{`"28.88"`, `28.88`} {
			t.Run(fmt.Sprintf("status_%d_money_%s", status, money), func(t *testing.T) {
				provider := bepTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, "/api/v1/pay/info", r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					var payload map[string]any
					require.NoError(t, json.Unmarshal(body, &payload))
					require.Equal(t, "trade-1", payload["trade_id"])
					require.Equal(t, bepUpstreamSignature(t, body), payload["signature"])
					_, _ = fmt.Fprintf(w, `{"status_code":200,"data":{"trade_id":"trade-1","status":%d,"money":%s,"actual_amount":"4.25","fiat":"CNY"}}`, status, money)
				})
				resp, err := provider.QueryOrder(context.Background(), "trade-1")
				require.NoError(t, err)
				require.Equal(t, expected, resp.Status)
				require.Equal(t, 28.88, resp.Amount)
				require.Equal(t, "4.25", resp.Metadata["actual_amount"])
				require.Equal(t, "CNY", resp.Metadata["currency"])
			})
		}
	}
}

func TestBepusdtQueryRejectsUnsafePaymentEvidence(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(data map[string]any) { data["money"] = "NaN" },
		func(data map[string]any) { data["money"] = "Inf" },
		func(data map[string]any) { data["money"] = 0 },
		func(data map[string]any) { data["money"] = -1 },
		func(data map[string]any) { delete(data, "money"); data["amount"] = "4.25" },
		func(data map[string]any) { data["fiat"] = "USD" },
		func(data map[string]any) { delete(data, "fiat") },
		func(data map[string]any) { data["trade_id"] = "another-trade" },
		func(data map[string]any) { delete(data, "trade_id") },
		func(data map[string]any) { data["status"] = 0 },
		func(data map[string]any) { data["status"] = 7 },
		func(data map[string]any) { data["actual_amount"] = "NaN" },
	} {
		data := map[string]any{"trade_id": "trade-1", "money": "28.88", "fiat": "CNY", "status": 2, "actual_amount": "4.25"}
		mutate(data)
		provider := bepTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"status_code": 200, "data": data}))
		})
		_, err := provider.QueryOrder(context.Background(), "trade-1")
		require.Error(t, err, "%v", data)
	}
}

func bepSignedNotify(t *testing.T, raw string) string {
	t.Helper()
	return strings.TrimSuffix(raw, "}") + `,"signature":"` + bepUpstreamSignature(t, []byte(raw)) + `"}`
}

func TestBepusdtNotificationMatchesUpstreamNumericAndStringSigning(t *testing.T) {
	provider, err := NewBepusdt("test", map[string]string{"apiBase": "https://pay.example", "token": "test-token"})
	require.NoError(t, err)
	for _, amount := range []string{`28.88`, `28.880`, `1000000`, `1e6`, `"28.880"`} {
		for _, actual := range []string{`4.25`, `"4.2500"`} {
			t.Run(amount+"_"+actual, func(t *testing.T) {
				raw := bepSignedNotify(t, fmt.Sprintf(`{"trade_id":"trade-1","order_id":"order-1","amount":%s,"actual_amount":%s,"status":2,"token":"wallet","block_transaction_id":"tx"}`, amount, actual))
				notification, err := provider.VerifyNotification(context.Background(), raw, nil)
				require.NoError(t, err)
				require.Equal(t, "trade-1", notification.TradeNo)
				require.Equal(t, "order-1", notification.OrderID)
				require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
				require.Equal(t, raw, notification.RawData)
				require.Equal(t, "4.25", notification.Metadata["actual_amount"])
				require.Equal(t, "CNY", notification.Metadata["currency"])
			})
		}
	}
}

func TestBepusdtNotificationRejectsTamperingAndInvalidFields(t *testing.T) {
	provider, err := NewBepusdt("test", map[string]string{"apiBase": "https://pay.example", "token": "test-token"})
	require.NoError(t, err)
	valid := `{"trade_id":"trade-1","order_id":"order-1","amount":28.88,"actual_amount":"4.25","status":2}`
	signed := bepSignedNotify(t, valid)
	for _, raw := range []string{
		valid, strings.Replace(signed, `28.88`, `288.88`, 1),
		strings.Replace(signed, `order-1`, `order-2`, 1),
		strings.Replace(signed, `"status":2`, `"status":1`, 1),
		signed + `{}`, `null`, `{`,
	} {
		_, err := provider.VerifyNotification(context.Background(), raw, nil)
		require.Error(t, err, raw)
	}
	for _, mutate := range []func(map[string]any){
		func(data map[string]any) { delete(data, "trade_id") },
		func(data map[string]any) { data["order_id"] = " " },
		func(data map[string]any) { data["amount"] = "NaN" },
		func(data map[string]any) { data["amount"] = "-Inf" },
		func(data map[string]any) { data["amount"] = 0 },
		func(data map[string]any) { data["amount"] = -1 },
		func(data map[string]any) { data["fiat"] = "USD" },
		func(data map[string]any) { data["status"] = 7 },
		func(data map[string]any) { delete(data, "status") },
		func(data map[string]any) { data["actual_amount"] = "NaN" },
	} {
		var data map[string]any
		require.NoError(t, json.Unmarshal([]byte(valid), &data))
		mutate(data)
		body, err := json.Marshal(data)
		require.NoError(t, err)
		_, err = provider.VerifyNotification(context.Background(), bepSignedNotify(t, string(body)), nil)
		require.Error(t, err, string(body))
	}
}

func TestBepusdtNotificationDoesNotTreatConfirmingOrCancelledAsPaid(t *testing.T) {
	provider, err := NewBepusdt("test", map[string]string{"apiBase": "https://pay.example", "token": "test-token"})
	require.NoError(t, err)
	for status, want := range map[int]string{
		1: payment.ProviderStatusPending, 3: payment.ProviderStatusFailed,
		4: payment.ProviderStatusFailed, 5: payment.ProviderStatusPending, 6: payment.ProviderStatusFailed,
	} {
		raw := bepSignedNotify(t, fmt.Sprintf(`{"trade_id":"trade-1","order_id":"order-1","amount":28.88,"actual_amount":"4.25","status":%d}`, status))
		notification, err := provider.VerifyNotification(context.Background(), raw, nil)
		require.NoError(t, err)
		require.Equal(t, want, notification.Status)
	}
}

func TestBepusdtQueryRejectsBusinessErrorsAndMalformedJSON(t *testing.T) {
	for _, body := range []string{`{"status_code":400,"message":"order not found"}`, `{`, `null`} {
		provider := bepTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		})
		_, err := provider.QueryOrder(context.Background(), "trade-1")
		require.Error(t, err)
	}
}

func TestBepusdtCancelHonorsBusinessErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		response  string
		wantError bool
	}{
		{"cancelled", `{"status_code":200,"data":{"trade_id":"trade-1"}}`, false},
		{"already paid", `{"status_code":400,"message":"status does not allow cancellation"}`, true},
		{"malformed", `{`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := bepTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/order/cancel-transaction", r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Equal(t, "trade-1", payload["trade_id"])
				require.Equal(t, bepUpstreamSignature(t, body), payload["signature"])
				_, _ = io.WriteString(w, tc.response)
			})
			err := provider.CancelPayment(context.Background(), "trade-1")
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type bepRoundTripper func(*http.Request) (*http.Response, error)

func (f bepRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type bepBrokenBody struct{}

func (bepBrokenBody) Read([]byte) (int, error) { return 0, errors.New("broken body") }
func (bepBrokenBody) Close() error             { return nil }

func TestBepusdtHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   io.ReadCloser
		want   string
	}{
		{"http error", 503, io.NopCloser(strings.NewReader("unavailable")), "HTTP 503"},
		{"oversized", 200, io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1))), "exceeds"},
		{"read failure", 200, bepBrokenBody{}, "broken body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewBepusdt("test", map[string]string{"apiBase": "https://pay.example", "token": "test-token"})
			require.NoError(t, err)
			provider.client.Transport = bepRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: tc.body}, nil
			})
			_, err = provider.QueryOrder(context.Background(), "trade-1")
			require.ErrorContains(t, err, tc.want)
		})
	}
	provider := bepTestProvider(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid JSON reached upstream") })
	_, err := provider.postJSON(context.Background(), "/api/v1/pay/info", map[string]any{"amount": math.NaN()})
	require.ErrorContains(t, err, "encode request")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.QueryOrder(ctx, "trade-1")
	require.ErrorIs(t, err, context.Canceled)
}
