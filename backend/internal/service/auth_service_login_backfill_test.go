//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldApplyEmailFirstBindDefaults_NilIdentityReturnsFalse(t *testing.T) {
	svc := &AuthService{}

	require.False(t, svc.shouldApplyEmailFirstBindDefaults(context.Background(), 1, nil, false))
}
