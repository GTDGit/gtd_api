package models

import (
    "encoding/json"
    "time"
)

// TransactionLog stores upstream transaction request/response log entries.
type TransactionLog struct {
    ID            int             `db:"id"`
    TransactionID int             `db:"transaction_id"`
    SkuID         int             `db:"sku_id"`
    DigiRefID     string          `db:"digi_ref_id"`
    Request       json.RawMessage `db:"request"`
    Response      json.RawMessage `db:"response"`
    RC            *string         `db:"rc"`
    Status        *string         `db:"status"`
    CreatedAt     time.Time       `db:"created_at"`
    ResponseAt    *time.Time      `db:"response_at"`
}

// CallbackLog stores outgoing webhook attempts to client systems.
type CallbackLog struct {
    ID            int             `db:"id"`
    TransactionID int             `db:"transaction_id"`
    ClientID      int             `db:"client_id"`
    Event         string          `db:"event"`
    Payload       json.RawMessage `db:"payload"`
    Attempt       int             `db:"attempt"`
    HTTPStatus    *int            `db:"http_status"`
    ResponseBody  *string         `db:"response_body"`
    IsDelivered   bool            `db:"is_delivered"`
    CreatedAt     time.Time       `db:"created_at"`
    NextRetryAt   *time.Time      `db:"next_retry_at"`
}

// DigiflazzCallback stores raw callback payload from Digiflazz.
type DigiflazzCallback struct {
    ID           int             `db:"id"`
    DigiRefID    string          `db:"digi_ref_id"`
    Payload      json.RawMessage `db:"payload"`
    RC           *string         `db:"rc"`
    Status       *string         `db:"status"`
    SerialNumber *string         `db:"serial_number"`
    IsProcessed  bool            `db:"is_processed"`
    ProcessedAt  *time.Time      `db:"processed_at"`
    CreatedAt    time.Time       `db:"created_at"`
}
