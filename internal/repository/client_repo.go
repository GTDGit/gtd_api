package repository

import (
    "database/sql"

    "github.com/jmoiron/sqlx"
    "github.com/lib/pq"

    "github.com/GTDGit/gtd_api/internal/models"
)

// ClientRepository provides data access methods for clients table.
type ClientRepository struct {
    db *sqlx.DB
}

// NewClientRepository creates a new ClientRepository.
func NewClientRepository(db *sqlx.DB) *ClientRepository {
    return &ClientRepository{db: db}
}

// getBy is a small helper to fetch a single client by a specific column
// using a prepared statement. It ensures ip_whitelist is scanned via pq.Array.
func (r *ClientRepository) getBy(where string, arg any) (*models.Client, error) {
    const base = `SELECT id, client_id, name, api_key, sandbox_key, callback_url, callback_secret,
        ip_whitelist, is_active, created_at, updated_at
        FROM clients WHERE `

    stmt, err := r.db.Preparex(base + where + " LIMIT 1")
    if err != nil {
        return nil, err
    }
    defer stmt.Close()

    row := stmt.QueryRowx(arg)
    var c models.Client
    // Explicit scan to use pq.Array for the TEXT[] field as required.
    if err := row.Scan(
        &c.ID,
        &c.ClientID,
        &c.Name,
        &c.APIKey,
        &c.SandboxKey,
        &c.CallbackURL,
        &c.CallbackSecret,
        pq.Array(&c.IPWhitelist),
        &c.IsActive,
        &c.CreatedAt,
        &c.UpdatedAt,
    ); err != nil {
        if err == sql.ErrNoRows {
            return nil, sql.ErrNoRows
        }
        return nil, err
    }
    return &c, nil
}

// GetByAPIKey finds a client by production API key.
func (r *ClientRepository) GetByAPIKey(apiKey string) (*models.Client, error) {
    return r.getBy("api_key = $1", apiKey)
}

// GetBySandboxKey finds a client by sandbox key.
func (r *ClientRepository) GetBySandboxKey(sandboxKey string) (*models.Client, error) {
    return r.getBy("sandbox_key = $1", sandboxKey)
}

// GetByClientID finds a client by public client identifier.
func (r *ClientRepository) GetByClientID(clientID string) (*models.Client, error) {
    return r.getBy("client_id = $1", clientID)
}

// GetByID finds a client by numeric id.
func (r *ClientRepository) GetByID(id int) (*models.Client, error) {
    return r.getBy("id = $1", id)
}

// Create creates a new client.
func (r *ClientRepository) Create(client *models.Client) error {
    query := `INSERT INTO clients (client_id, name, api_key, sandbox_key, callback_url, callback_secret, ip_whitelist, is_active)
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
              RETURNING id, created_at, updated_at`

    return r.db.QueryRowx(query,
        client.ClientID,
        client.Name,
        client.APIKey,
        client.SandboxKey,
        client.CallbackURL,
        client.CallbackSecret,
        pq.Array(client.IPWhitelist),
        client.IsActive,
    ).Scan(&client.ID, &client.CreatedAt, &client.UpdatedAt)
}

// Update updates an existing client.
func (r *ClientRepository) Update(client *models.Client) error {
    query := `UPDATE clients
              SET name = $1, callback_url = $2, callback_secret = $3,
                  ip_whitelist = $4, is_active = $5, api_key = $6, sandbox_key = $7
              WHERE id = $8
              RETURNING updated_at`

    return r.db.QueryRowx(query,
        client.Name,
        client.CallbackURL,
        client.CallbackSecret,
        pq.Array(client.IPWhitelist),
        client.IsActive,
        client.APIKey,
        client.SandboxKey,
        client.ID,
    ).Scan(&client.UpdatedAt)
}

// List retrieves all clients.
func (r *ClientRepository) List() ([]*models.Client, error) {
    query := `SELECT id, client_id, name, api_key, sandbox_key, callback_url, callback_secret,
                     ip_whitelist, is_active, created_at, updated_at
              FROM clients
              ORDER BY created_at DESC`

    rows, err := r.db.Queryx(query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var clients []*models.Client
    for rows.Next() {
        var c models.Client
        if err := rows.Scan(
            &c.ID,
            &c.ClientID,
            &c.Name,
            &c.APIKey,
            &c.SandboxKey,
            &c.CallbackURL,
            &c.CallbackSecret,
            pq.Array(&c.IPWhitelist),
            &c.IsActive,
            &c.CreatedAt,
            &c.UpdatedAt,
        ); err != nil {
            return nil, err
        }
        clients = append(clients, &c)
    }

    return clients, rows.Err()
}
