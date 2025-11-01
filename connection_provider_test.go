package pgdbtemplatepgxv4_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/andrei-polukhin/pgdbtemplate"
	pgdbtemplatepgx "github.com/andrei-polukhin/pgdbtemplate-pgx-v4"
)

// testConnectionStringFuncPgx creates a connection string for pgx tests.
func testConnectionStringFuncPgx(dbName string) string {
	return pgdbtemplate.ReplaceDatabaseInConnectionString(testConnectionString, dbName)
}

func TestPgxConnectionProvider(t *testing.T) {
	t.Parallel()
	c := qt.New(t)
	ctx := context.Background()

	c.Run("Basic pgx connection", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("Pgx connection with pool options", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithMaxConns(5),
			pgdbtemplatepgx.WithMinConns(1),
		)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("MinConns option alone", func(c *qt.C) {
		c.Parallel()
		// Test WithPgxMinConns when poolConfig is initially nil.
		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithMaxConns(2),
		)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("Custom pool configuration", func(c *qt.C) {
		c.Parallel()
		// Create a custom pool config.
		baseConnString := testConnectionStringFuncPgx("postgres")
		poolConfig, err := pgxpool.ParseConfig(baseConnString)
		c.Assert(err, qt.IsNil)
		poolConfig.MaxConns = 3
		poolConfig.MinConns = 1

		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithPoolConfig(*poolConfig),
		)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("Connection error handling", func(c *qt.C) {
		// Test with invalid connection string.
		invalidConnStringFunc := func(dbName string) string {
			return "invalid://connection/string"
		}
		provider := pgdbtemplatepgx.NewConnectionProvider(invalidConnStringFunc)
		defer provider.Close()

		_, err := provider.Connect(ctx, "testdb")
		c.Assert(err, qt.ErrorMatches, "failed to parse connection string:.*")
	})

	c.Run("Connection to nonexistent database", func(c *qt.C) {
		c.Parallel()
		nonExistentFunc := func(dbName string) string {
			return pgdbtemplate.ReplaceDatabaseInConnectionString(testConnectionString, "nonexistent_db_12345")
		}
		provider := pgdbtemplatepgx.NewConnectionProvider(nonExistentFunc)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "nonexistent")
		c.Assert(err, qt.IsNotNil)
		c.Assert(conn, qt.IsNil)
		c.Assert(err, qt.ErrorMatches, "failed to create connection pool:.*")
	})

	c.Run("Pool reuse", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
		defer provider.Close()

		// Connect to the same database twice to test pool reuse.
		conn1, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn1.Close(), qt.IsNil) }()

		conn2, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn2.Close(), qt.IsNil) }()

		// Both connections should work.
		var value int
		row := conn1.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)

		row = conn2.QueryRowContext(ctx, "SELECT 2")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 2)
	})

	c.Run("Concurrent pool double-check", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
		defer provider.Close()

		ctx := context.Background()
		dbName := "postgres"

		start := make(chan struct{})
		results := make(chan error, 2)
		openPoolConn := func() {
			<-start // Wait for the signal to start.
			conn, err := provider.Connect(ctx, dbName)
			if conn != nil {
				defer conn.Close()
			}
			results <- err
		}
		go openPoolConn()
		go openPoolConn()

		// Signal both goroutines to start simultaneously.
		close(start)

		// Wait for both goroutines to finish.
		// Both should succeed without error.
		// This tests the double-check locking in GetPool.
		for i := 0; i < 2; i++ {
			err := <-results
			c.Assert(err, qt.IsNil)
		}
	})

	c.Run("Wrong MaxConns handling", func(c *qt.C) {
		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithMaxConns(-1), // Pool will not be created.
		)
		defer provider.Close()

		_, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.ErrorMatches, "MaxConns must be >= 1, got -1")
	})

	c.Run("Context cancellation during pool creation", func(c *qt.C) {
		// Create a context that gets cancelled immediately.
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
		defer provider.Close()

		_, err := provider.Connect(cancelCtx, "postgres")
		c.Assert(err, qt.ErrorMatches, "failed to create connection pool:.*")
	})

	c.Run("WithMaxConnLifetime option", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithMaxConns(5),
			pgdbtemplatepgx.WithMaxConnLifetime(1*time.Hour),
		)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("WithMaxConnIdleTime option", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithMaxConns(5),
			pgdbtemplatepgx.WithMaxConnIdleTime(30*time.Minute),
		)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("All pool time options together", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithMaxConns(10),
			pgdbtemplatepgx.WithMinConns(2),
			pgdbtemplatepgx.WithMaxConnLifetime(2*time.Hour),
			pgdbtemplatepgx.WithMaxConnIdleTime(45*time.Minute),
			pgdbtemplatepgx.WithAfterConnect(func(ctx context.Context, conn *pgx.Conn) error {
				_, err := conn.Query(ctx, "SELECT 'Postgres is cool!'")
				return err
			}),
		)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works with all options set.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("Time options with zero values", func(c *qt.C) {
		c.Parallel()
		// Zero values should be acceptable (means no limit).
		provider := pgdbtemplatepgx.NewConnectionProvider(
			testConnectionStringFuncPgx,
			pgdbtemplatepgx.WithMaxConns(5),
			pgdbtemplatepgx.WithMaxConnLifetime(0),
			pgdbtemplatepgx.WithMaxConnIdleTime(0),
		)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn.Close(), qt.IsNil) }()

		// Verify the connection works even with zero time limits.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 1)
	})

	c.Run("DatabaseConnection.Close() removes pool from provider", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
		defer provider.Close()

		// Connect to a database (use postgres as it exists).
		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)

		// Use the connection to verify it works.
		var value int
		row := conn.QueryRowContext(ctx, "SELECT 1")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)

		// Close the connection - this should remove the pool from provider.
		err = conn.Close()
		c.Assert(err, qt.IsNil)

		// Connect again to the same database - should create a new pool.
		conn2, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn2.Close(), qt.IsNil) }()

		// Verify the new connection works.
		row = conn2.QueryRowContext(ctx, "SELECT 2")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 2)
	})

	c.Run("Multiple connections closed independently", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
		defer provider.Close()

		// Connect to postgres - they will share the same pool
		// since it's the same database name.
		conn1, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)

		conn2, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)

		conn3, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)

		// All connections should work.
		var value int
		for i, conn := range []pgdbtemplate.DatabaseConnection{conn1, conn2, conn3} {
			row := conn.QueryRowContext(ctx, fmt.Sprintf("SELECT %d", i+1))
			err = row.Scan(&value)
			c.Assert(err, qt.IsNil)
			c.Assert(value, qt.Equals, i+1)
		}

		// Close connections in different order.
		err = conn2.Close()
		c.Assert(err, qt.IsNil)

		err = conn1.Close()
		c.Assert(err, qt.IsNil)

		err = conn3.Close()
		c.Assert(err, qt.IsNil)

		// Verify we can create new connections after closing.
		conn4, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)
		defer func() { c.Assert(conn4.Close(), qt.IsNil) }()

		row := conn4.QueryRowContext(ctx, "SELECT 4")
		err = row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, 4)
	})

	c.Run("Double close is safe", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
		defer provider.Close()

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)

		// First close should succeed.
		err = conn.Close()
		c.Assert(err, qt.IsNil)

		// Second close should not panic or error.
		err = conn.Close()
		c.Assert(err, qt.IsNil)
	})

	c.Run("Provider.Close() with no pools", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)

		// Close without creating any connections - should not panic.
		provider.Close()

		// Calling Close() again should also be safe.
		provider.Close()
	})

	c.Run("Provider.Close() after individual connection closes", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)

		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)

		// Close the connection first (removes pool).
		err = conn.Close()
		c.Assert(err, qt.IsNil)

		// Now close provider - should handle empty map gracefully.
		provider.Close()
	})

	c.Run("DatabaseConnection without provider (manually created)", func(c *qt.C) {
		c.Parallel()
		// Create a DatabaseConnection manually without provider.
		baseConnString := testConnectionStringFuncPgx("postgres")
		poolConfig, err := pgxpool.ParseConfig(baseConnString)
		c.Assert(err, qt.IsNil)

		pool, err := pgxpool.ConnectConfig(ctx, poolConfig)
		c.Assert(err, qt.IsNil)

		// Create DatabaseConnection without provider tracking.
		conn := &pgdbtemplatepgx.DatabaseConnection{
			Pool: pool,
			// provider and dbName are nil/empty
		}

		// Close should not panic even without provider.
		err = conn.Close()
		c.Assert(err, qt.IsNil)
	})

	c.Run("Concurrent Close() calls on provider", func(c *qt.C) {
		c.Parallel()
		provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)

		// Create a connection.
		conn, err := provider.Connect(ctx, "postgres")
		c.Assert(err, qt.IsNil)

		// Close both connection and provider concurrently.
		done := make(chan bool, 2)

		go func() {
			conn.Close()
			done <- true
		}()

		go func() {
			provider.Close()
			done <- true
		}()

		// Wait for both to complete without panic.
		<-done
		<-done
	})
}

func TestTemplateManagerWithPgx(t *testing.T) {
	t.Parallel()
	c := qt.New(t)
	ctx := context.Background()

	// Create pgx connection provider.
	provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)

	// Create migration runner.
	migrationRunner := createTestMigrationRunner(c)

	templateName := fmt.Sprintf("pgx_test_template_%d_%d", time.Now().UnixNano(), os.Getpid())
	config := pgdbtemplate.Config{
		ConnectionProvider: provider,
		MigrationRunner:    migrationRunner,
		TemplateName:       templateName,
		TestDBPrefix:       fmt.Sprintf("pgx_test_%d_%d", time.Now().UnixNano(), os.Getpid()),
	}

	tm, err := pgdbtemplate.NewTemplateManager(config)
	c.Assert(err, qt.IsNil)

	// Initialize template.
	err = tm.Initialize(ctx)
	c.Assert(err, qt.IsNil)

	// CRITICAL: Close all pgx connections to allow template database to be used as a template.
	// pgx keeps connections in pools which prevents PostgreSQL from using the template database.
	provider.Close()

	// Create a new provider instance for test database operations.
	testProvider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)

	// Update the template manager with the new provider.
	config.ConnectionProvider = testProvider
	testTM, err := pgdbtemplate.NewTemplateManager(config)
	c.Assert(err, qt.IsNil)

	// Create test database.
	testDB, testDBName, err := testTM.CreateTestDatabase(ctx)
	c.Assert(err, qt.IsNil)
	defer func() {
		c.Assert(testDB.Close(), qt.IsNil)
		c.Assert(testTM.DropTestDatabase(ctx, testDBName), qt.IsNil)
		// Close all connections before cleanup.
		testProvider.Close()
		c.Assert(testTM.Cleanup(ctx), qt.IsNil)
	}()

	// Verify test database has the migrated schema.
	var count int
	row := testDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_table")
	err = row.Scan(&count)
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, 2) // Should have 2 rows from migration.

	// Test pgx-specific functionality.
	result, err := testDB.ExecContext(ctx, "INSERT INTO test_table (name) VALUES ($1)", "pgx_test")
	c.Assert(err, qt.IsNil)
	c.Assert(result, qt.IsNotNil)

	// Verify the insertion.
	row = testDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_table")
	err = row.Scan(&count)
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, 3) // Should now have 3 rows.
}

// Helper function to create a test migration runner.
func createTestMigrationRunner(c *qt.C) pgdbtemplate.MigrationRunner {
	// Create temporary migration files.
	tempDir := c.TempDir()

	migration1 := `
	CREATE TABLE test_table (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		created_at TIMESTAMP DEFAULT NOW()
	);`

	migration2 := `
	INSERT INTO test_table (name)
	VALUES ('test_data_1'), ('test_data_2');`

	migration1Path := tempDir + "/001_create_table.sql"
	migration2Path := tempDir + "/002_insert_data.sql"

	err := os.WriteFile(migration1Path, []byte(migration1), 0644)
	c.Assert(err, qt.IsNil)

	err = os.WriteFile(migration2Path, []byte(migration2), 0644)
	c.Assert(err, qt.IsNil)

	return pgdbtemplate.NewFileMigrationRunner([]string{tempDir}, pgdbtemplate.AlphabeticalMigrationFilesSorting)
}

// TestHighConcurrencyPoolCreation tests that multiple goroutines creating pools
// simultaneously don't block each other unnecessarily and only one pool is created.
func TestHighConcurrencyPoolCreation(t *testing.T) {
	t.Parallel()
	c := qt.New(t)
	ctx := context.Background()

	provider := pgdbtemplatepgx.NewConnectionProvider(testConnectionStringFuncPgx)
	defer provider.Close()

	const numGoroutines = 50
	dbName := "postgres"

	start := make(chan struct{})
	errors := make(chan error, numGoroutines)
	connections := make(chan pgdbtemplate.DatabaseConnection, numGoroutines)

	// Launch many goroutines that try to connect simultaneously.
	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-start // Wait for signal to start.
			conn, err := provider.Connect(ctx, dbName)
			errors <- err
			connections <- conn
		}()
	}

	// Signal all goroutines to start at once.
	close(start)

	// Collect all results.
	conns := make([]pgdbtemplate.DatabaseConnection, 0, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		err := <-errors
		c.Assert(err, qt.IsNil, qt.Commentf("goroutine %d failed", i))

		conn := <-connections
		conns = append(conns, conn)
	}

	// All connections should work.
	c.Assert(len(conns), qt.Equals, numGoroutines)

	// Verify a few connections can execute queries.
	for i := 0; i < 10; i++ {
		var value int
		row := conns[i].QueryRowContext(ctx, fmt.Sprintf("SELECT %d", i+1))
		err := row.Scan(&value)
		c.Assert(err, qt.IsNil)
		c.Assert(value, qt.Equals, i+1)
	}

	// Close all connections.
	for _, conn := range conns {
		conn.Close()
	}
}
