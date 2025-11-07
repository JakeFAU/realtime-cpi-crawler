// Package database_test contains unit tests for the database package.
package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logging.InitLogger()
}

// MockConnector is a mock implementation of the Connector interface.
// It allows for mocking the Connect method.
type MockConnector struct {
	DB  *sqlx.DB
	Err error
}

// Connect returns the mock DB and error.
func (m *MockConnector) Connect(_ context.Context, _, _ string) (*sqlx.DB, error) {
	return m.DB, m.Err
}

func TestNewPostgresProvider_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer mockDB.Close() //nolint:errcheck

	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
	mock.ExpectPing()

	connector := &MockConnector{DB: sqlxDB}

	provider, err := database.NewPostgresProvider(context.Background(), "", connector)
	require.NoError(t, err)
	assert.NotNil(t, provider)

	err = mock.ExpectationsWereMet()
	assert.NoError(t, err, "sqlmock expectations not met")
}

func TestNewPostgresProvider_ConnectError(t *testing.T) {
	connectErr := errors.New("connect failed")
	connector := &MockConnector{Err: connectErr}

	_, err := database.NewPostgresProvider(context.Background(), "", connector)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to postgres")
}

func TestNewPostgresProvider_PingError(t *testing.T) {
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer mockDB.Close() //nolint:errcheck

	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")

	pingErr := errors.New("ping failed")
	mock.ExpectPing().WillReturnError(pingErr)

	connector := &MockConnector{DB: sqlxDB}

	_, err = database.NewPostgresProvider(context.Background(), "", connector)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ping postgres")

	err = mock.ExpectationsWereMet()
	assert.NoError(t, err, "sqlmock expectations not met")
}
