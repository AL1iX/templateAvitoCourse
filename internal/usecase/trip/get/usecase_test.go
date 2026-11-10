package get

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

func TestExecute(t *testing.T) {
	found := trip.Restore("t1", "u1", "d1", trip.Point{}, trip.Point{}, 100,
		trip.StatusActive, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), nil)
	errDB := errors.New("db down")

	tests := []struct {
		name    string
		repoRet *trip.Trip
		repoErr error
		wantErr error
	}{
		{name: "found", repoRet: found},
		{name: "not found", repoErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "repository error", repoErr: errDB, wantErr: errDB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			repo.EXPECT().GetByID(gomock.Any(), "t1").Return(tc.repoRet, tc.repoErr)

			got, err := New(repo).Execute(context.Background(), "t1")

			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.repoRet, got)
		})
	}
}
