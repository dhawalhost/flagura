package store

import (
	"context"

	"github.com/dhawalhost/flagura/pkg/domain"
)

type Store interface {
	// Flags & Audit
	ListFlags(ctx context.Context) ([]domain.FeatureFlag, error)
	GetFlag(ctx context.Context, keyOrID string) (*domain.FeatureFlag, error)
	SaveFlag(ctx context.Context, flag domain.FeatureFlag, actor string) (*domain.AuditLogEntry, error)
	DeleteFlag(ctx context.Context, keyOrID string, actor string) (*domain.AuditLogEntry, error)
	ToggleFlag(ctx context.Context, keyOrID string, env domain.Environment, enabled *bool, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error)
	UpdateRollout(ctx context.Context, keyOrID string, env domain.Environment, pct float64, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error)
	ListAuditLogs(ctx context.Context, limit int) ([]domain.AuditLogEntry, error)
	Reset(ctx context.Context) error
	Ping(ctx context.Context) error
	DriverName() string

	// Users & Authentication
	CreateUser(ctx context.Context, user domain.User) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	CreateSession(ctx context.Context, session domain.Session) error
	GetSession(ctx context.Context, token string) (*domain.Session, error)
	DeleteSession(ctx context.Context, token string) error
}
