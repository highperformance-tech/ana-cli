package harness

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/transport"
)

// guardOrg issues GetOrganization against the live endpoint and refuses to
// continue if orgId doesn't match wantID. This is the single hardest
// safeguard against pointing the test suite at the wrong tenant. The UUID
// orgId is used (vs the display name) so renames of the tenant don't
// silently widen the blast radius.
func guardOrg(ctx context.Context, c *transport.Client, wantID string) error {
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
	if resp.Organization.OrgID != wantID {
		return fmt.Errorf("expected orgId %q, got orgId=%s (name=%q) — refusing to mutate",
			wantID, resp.Organization.OrgID, resp.Organization.OrganizationName)
	}
	return nil
}
