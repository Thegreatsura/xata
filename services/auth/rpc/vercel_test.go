package rpc

import (
	"context"
	"errors"
	"testing"

	authv1 "xata/gen/proto/auth/v1"
	"xata/services/auth/store"
	storeMocks "xata/services/auth/store/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeVercelVerifier stands in for the saas Vercel token verifier.
type fakeVercelVerifier struct {
	installationID string
	err            error
}

func (f fakeVercelVerifier) Verify(string) (string, error) {
	return f.installationID, f.err
}

func TestResolveVercelInstallation(t *testing.T) {
	const installationID = "icfg_1"

	tests := map[string]struct {
		verifier     VercelTokenVerifier
		token        string
		installation string
		storeReturn  *store.VercelInstallation
		storeErr     error
		wantCode     codes.Code
		wantOrg      string
		wantStatus   string
	}{
		"not configured returns unimplemented": {
			verifier:     nil,
			token:        "tok",
			installation: installationID,
			wantCode:     codes.Unimplemented,
		},
		"missing token is invalid argument": {
			verifier:     fakeVercelVerifier{installationID: installationID},
			token:        "",
			installation: installationID,
			wantCode:     codes.InvalidArgument,
		},
		"missing installation is invalid argument": {
			verifier:     fakeVercelVerifier{installationID: installationID},
			token:        "tok",
			installation: "",
			wantCode:     codes.InvalidArgument,
		},
		"invalid token is unauthenticated": {
			verifier:     fakeVercelVerifier{err: errors.New("bad signature")},
			token:        "tok",
			installation: installationID,
			wantCode:     codes.Unauthenticated,
		},
		"token scoped to another installation is permission denied": {
			verifier:     fakeVercelVerifier{installationID: "icfg_other"},
			token:        "tok",
			installation: installationID,
			wantCode:     codes.PermissionDenied,
		},
		"token without an installation claim is permission denied": {
			verifier:     fakeVercelVerifier{installationID: ""},
			token:        "tok",
			installation: installationID,
			wantCode:     codes.PermissionDenied,
		},
		"missing installation is not found": {
			verifier:     fakeVercelVerifier{installationID: installationID},
			token:        "tok",
			installation: installationID,
			storeErr:     store.ErrVercelInstallationNotFound{InstallationID: installationID},
			wantCode:     codes.NotFound,
		},
		"store failure surfaces as unknown": {
			verifier:     fakeVercelVerifier{installationID: installationID},
			token:        "tok",
			installation: installationID,
			storeErr:     errors.New("db unavailable"),
			wantCode:     codes.Unknown,
		},
		"resolves the org and status": {
			verifier:     fakeVercelVerifier{installationID: installationID},
			token:        "tok",
			installation: installationID,
			storeReturn:  &store.VercelInstallation{XataOrganizationID: "org-1", Status: store.VercelInstallationDeleting},
			wantOrg:      "org-1",
			wantStatus:   string(store.VercelInstallationDeleting),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockStore := storeMocks.NewAuthStore(t)
			// The store is only consulted once the token verifies and scopes.
			if tc.storeReturn != nil || tc.storeErr != nil {
				mockStore.EXPECT().GetVercelInstallation(mock.Anything, tc.installation).
					Return(tc.storeReturn, tc.storeErr).Once()
			}

			svc := &AuthService{store: mockStore, vercelVerifier: tc.verifier}
			got, err := svc.ResolveVercelInstallation(context.Background(), &authv1.ResolveVercelInstallationRequest{
				Token:          tc.token,
				InstallationId: tc.installation,
			})

			if tc.wantCode != codes.OK {
				require.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantOrg, got.GetXataOrganizationId())
			require.Equal(t, tc.wantStatus, got.GetStatus())
		})
	}
}
