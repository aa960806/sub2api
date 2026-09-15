//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type bepusdtNetworkUserRepo struct {
	UserRepository
	user *User
}

func (r *bepusdtNetworkUserRepo) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}

func TestBepusdtAdditionalNetworkCheckoutAndOrder(t *testing.T) {
	for _, tc := range []struct{ method, tradeType string }{
		{payment.TypeBepusdtERC20, "usdt.erc20"},
		{payment.TypeBepusdtTON, "usdt.ton"},
	} {
		t.Run(tc.method, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			var upstreamRequests int
			var queryRequests int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/pay/info" {
					queryRequests++
					_, _ = fmt.Fprint(w, `{"status_code":200,"data":{"trade_id":"network-test","status":1,"money":10.5,"fiat":"CNY"}}`)
					return
				}
				upstreamRequests++
				require.Equal(t, "/api/v1/order/create-transaction", r.URL.Path)
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, tc.tradeType, payload["trade_type"])
				require.Equal(t, 10.5, payload["amount"], "existing recharge fee must remain applied")
				require.Equal(t, "CNY", payload["fiat"])
				require.Contains(t, payload["notify_url"], "/payment/webhook/bepusdt")
				_, _ = fmt.Fprint(w, `{"status_code":200,"data":{"trade_id":"network-test","payment_url":"https://pay.example/pay/network-test"}}`)
			}))
			t.Cleanup(upstream.Close)
			key := []byte("0123456789abcdef0123456789abcdef")
			configService := &PaymentConfigService{
				entClient:     client,
				encryptionKey: key,
				settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
					SettingPaymentEnabled: "true", SettingRechargeFeeRate: "5",
					SettingBalanceRechargeMult: "1",
				}},
			}
			config, err := configService.encryptConfig(map[string]string{
				"apiBase": upstream.URL, "token": "test-token", "tradeType": "usdt.bep20",
				"notifyUrl": "https://app.example.com/api/v1/payment/webhook/bepusdt",
			})
			require.NoError(t, err)
			inst, err := client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeBepusdt).SetName("USDT").SetConfig(config).
				SetSupportedTypes(payment.TypeBepusdt).SetPaymentMode("redirect").SetEnabled(true).
				SetLimits(`{"bepusdt":{"singleMin":2,"singleMax":100}}`).Save(ctx)
			require.NoError(t, err)
			user, err := client.User.Create().SetEmail("network@example.com").SetPasswordHash("hash").Save(ctx)
			require.NoError(t, err)
			svc := &PaymentService{
				entClient: client, configService: configService,
				loadBalancer:  payment.NewDefaultLoadBalancer(client, key),
				userRepo:      &bepusdtNetworkUserRepo{user: &User{ID: user.ID, Email: user.Email, Status: payment.EntityStatusActive}},
				resumeService: NewPaymentResumeService(key),
			}
			req := CreateOrderRequest{
				UserID: user.ID, Amount: 10, PaymentType: tc.method,
				OrderType: payment.OrderTypeBalance, SrcHost: "app.example.com",
				SrcURL: "https://app.example.com/purchase", ReturnURL: "https://app.example.com/payment/result",
			}

			limits, err := configService.GetAvailableMethodLimits(ctx)
			require.NoError(t, err)
			require.NotContains(t, limits.Methods, tc.method, "legacy config must not expose new networks")
			_, err = svc.CreateOrder(ctx, req)
			require.Error(t, err, "unconfigured networks must be rejected before creating an order")
			require.Zero(t, upstreamRequests)
			count, err := client.PaymentOrder.Query().Count(ctx)
			require.NoError(t, err)
			require.Zero(t, count)

			_, err = inst.Update().SetSupportedTypes(payment.TypeBepusdt + "," + tc.method).Save(ctx)
			require.NoError(t, err)
			limits, err = configService.GetAvailableMethodLimits(ctx)
			require.NoError(t, err)
			require.Contains(t, limits.Methods, tc.method)
			require.Equal(t, 2.0, limits.Methods[tc.method].SingleMin)
			require.Equal(t, 100.0, limits.Methods[tc.method].SingleMax)
			require.Equal(t, "CNY", limits.Methods[tc.method].Currency)

			resp, err := svc.CreateOrder(ctx, req)
			require.NoError(t, err)
			require.Equal(t, 1, upstreamRequests)
			require.Equal(t, tc.method, resp.PaymentType)
			require.Equal(t, 10.5, resp.PayAmount)
			order, err := client.PaymentOrder.Get(ctx, resp.OrderID)
			require.NoError(t, err)
			require.Equal(t, tc.method, order.PaymentType)
			require.Equal(t, payment.TypeBepusdt, order.ProviderSnapshot["provider_key"])
			require.Equal(t, strconv.FormatInt(inst.ID, 10), order.ProviderSnapshot["provider_instance_id"])
			require.NotContains(t, order.ProviderSnapshot, "token")
			claims, err := svc.resumeService.ParseToken(resp.ResumeToken)
			require.NoError(t, err)
			require.Equal(t, tc.method, claims.PaymentType)
			require.Equal(t, payment.TypeBepusdt, claims.ProviderKey)
			recovered, err := svc.GetPublicOrderByResumeToken(ctx, resp.ResumeToken)
			require.NoError(t, err)
			require.Equal(t, order.ID, recovered.ID)
			require.Equal(t, 1, queryRequests, "resume must query the pinned BEpusdt instance")
			walletOwner, err := client.User.Get(ctx, user.ID)
			require.NoError(t, err)
			require.Equal(t, user.Balance, walletOwner.Balance, "creating pending orders must not change user balances")
		})
	}
}
