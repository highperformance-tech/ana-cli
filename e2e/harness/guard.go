package harness

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/transport"
)

// guardOrg issues GetOrganization against the live endpoint and refuses to
// continue if the organizationName doesn't match want. This is the single
// hardest safeguard against pointing the test suite at the wrong tenant.
func guardOrg(ctx context.Context, c *transport.Client, want string) error {
	var resp struct {
		Organization struct {
			OrgID            string `json:"orgId"`
			OrganizationName string `json:"organizationName"`
		} `json:"organization"`
	}
	const path = "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization"
	if err := c.Unary(ctx, path, struct{}{}, &resp); err != nil {
		return fmt.Errorf("GetOrganization: %w", err)
	}
	if resp.Organization.OrganizationName != want {
		return fmt.Errorf("expected org %q, got %q (orgId=%s) — refusing to mutate",
			want, resp.Organization.OrganizationName, resp.Organization.OrgID)
	}
	return nil
}
