package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/transport"
)

// sweepPrefix matches every resource created by a harness run.
const sweepPrefix = "anacli-e2e-"

// sweepPrior deletes any connectors/chats/api-keys/service-accounts in the
// target org whose name starts with sweepPrefix. Called before a fresh test
// run so a previous crashed run's leftovers cannot perturb the new suite.
func sweepPrior(ctx context.Context, c *transport.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var errs []string
	if err := sweepConnectors(ctx, c); err != nil {
		errs = append(errs, fmt.Sprintf("connectors: %v", err))
	}
	if err := sweepChats(ctx, c); err != nil {
		errs = append(errs, fmt.Sprintf("chats: %v", err))
	}
	if err := sweepAPIKeys(ctx, c); err != nil {
		errs = append(errs, fmt.Sprintf("api keys: %v", err))
	}
	if err := sweepServiceAccounts(ctx, c); err != nil {
		errs = append(errs, fmt.Sprintf("service accounts: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("sweep: %s", strings.Join(errs, "; "))
	}
	return nil
}

func sweepConnectors(ctx context.Context, c *transport.Client) error {
	var resp struct {
		Connectors []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"connectors"`
	}
	if err := c.Unary(ctx, "/rpc/public/textql.rpc.public.connector.ConnectorService/GetConnectors", struct{}{}, &resp); err != nil {
		return err
	}
	for _, k := range resp.Connectors {
		if !strings.HasPrefix(k.Name, sweepPrefix) {
			continue
		}
		_ = c.Unary(ctx, "/rpc/public/textql.rpc.public.connector.ConnectorService/DeleteConnector",
			map[string]any{"connectorId": k.ID}, nil)
	}
	return nil
}

func sweepChats(ctx context.Context, c *transport.Client) error {
	var resp struct {
		Chats []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"chats"`
	}
	if err := c.Unary(ctx, "/rpc/public/textql.rpc.public.chat.ChatService/GetChats", struct{}{}, &resp); err != nil {
		return err
	}
	for _, ch := range resp.Chats {
		if !strings.HasPrefix(ch.Summary, sweepPrefix) {
			continue
		}
		_ = c.Unary(ctx, "/rpc/public/textql.rpc.public.chat.ChatService/DeleteChat",
			map[string]any{"chatId": ch.ID}, nil)
	}
	return nil
}

func sweepAPIKeys(ctx context.Context, c *transport.Client) error {
	var resp struct {
		APIKeys []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"apiKeys"`
	}
	if err := c.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListApiKeys", struct{}{}, &resp); err != nil {
		return err
	}
	for _, k := range resp.APIKeys {
		if !strings.HasPrefix(k.Name, sweepPrefix) {
			continue
		}
		_ = c.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/RevokeApiKey",
			map[string]any{"apiKeyId": k.ID}, nil)
	}
	return nil
}

func sweepServiceAccounts(ctx context.Context, c *transport.Client) error {
	var resp struct {
		ServiceAccounts []struct {
			MemberID    string `json:"memberId"`
			DisplayName string `json:"displayName"`
		} `json:"serviceAccounts"`
	}
	if err := c.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListServiceAccounts", struct{}{}, &resp); err != nil {
		return err
	}
	for _, sa := range resp.ServiceAccounts {
		if !strings.HasPrefix(sa.DisplayName, sweepPrefix) {
			continue
		}
		_ = c.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/DeleteServiceAccount",
			map[string]any{"memberId": sa.MemberID}, nil)
	}
	return nil
}
