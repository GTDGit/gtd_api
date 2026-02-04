-- ============================================
-- Migration 000007: Identity Tables (OCR, Liveness)
-- Note: Territory tables (provinces, cities, districts, sub_districts)
--       are created in separate migrations (000008-000011)
-- ============================================

-- OCR Requests
CREATE TABLE ocr_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ocr_id VARCHAR(50) NOT NULL UNIQUE,                 -- 'OCR-20250128-000001'
    client_id INT NOT NULL REFERENCES clients(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,

    -- Document info
    doc_type identity_doc_type NOT NULL,                 -- 'KTP', 'NPWP', 'SIM'
    image_url VARCHAR(500) NOT NULL,                     -- S3/storage URL of uploaded image

    -- Status
    status ocr_status NOT NULL DEFAULT 'Pending',

    -- Extracted data
    extracted_data JSONB,                                -- Full parsed data from OCR
    -- KTP: { nik, nama, tempatLahir, tanggalLahir, jenisKelamin, alamat, rt, rw, kelurahan, kecamatan, agama, statusPerkawinan, pekerjaan, kewarganegaraan, berlakuHingga }
    -- NPWP: { npwp, nama, alamat }
    -- SIM: { nomor, nama, tempatLahir, tanggalLahir, jenisKelamin, alamat, golonganDarah, pekerjaan, berlakuHingga, jenisSim }

    -- Confidence score
    confidence DECIMAL(5,2),                             -- Overall confidence (0-100)
    field_confidence JSONB,                              -- Per-field confidence scores

    -- Address matching (KTP only)
    matched_province_code VARCHAR(2),
    matched_city_code VARCHAR(4),
    matched_district_code VARCHAR(6),
    matched_sub_district_code VARCHAR(10),

    -- Provider info
    provider VARCHAR(50),                                -- 'aws_textract', 'google_vision', 'internal'
    provider_data JSONB,                                 -- Raw provider response

    -- Error
    failed_reason VARCHAR(255),

    -- Timing
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    processing_time_ms INT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ocr_requests_ocr_id ON ocr_requests(ocr_id);
CREATE INDEX idx_ocr_requests_client_id ON ocr_requests(client_id);
CREATE INDEX idx_ocr_requests_doc_type ON ocr_requests(doc_type);
CREATE INDEX idx_ocr_requests_status ON ocr_requests(status);
CREATE INDEX idx_ocr_requests_created_at ON ocr_requests(created_at);

-- ocr_records table for storing OCR extraction results
CREATE TABLE IF NOT EXISTS ocr_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id INTEGER NOT NULL REFERENCES clients(id),
    doc_type identity_doc_type NOT NULL, -- KTP, NPWP, SIM (uses existing enum)

    -- Common fields
    nik VARCHAR(16),
    full_name VARCHAR(255) NOT NULL,
    place_of_birth VARCHAR(100),
    date_of_birth VARCHAR(10),
    gender VARCHAR(10),
    blood_type VARCHAR(5),

    -- Address
    address JSONB,
    administrative_code JSONB,

    -- KTP specific
    religion VARCHAR(20),
    marital_status VARCHAR(30),
    occupation VARCHAR(100),
    nationality VARCHAR(5),
    valid_until VARCHAR(20),
    published_in VARCHAR(100),
    published_on VARCHAR(10),

    -- NPWP specific
    npwp VARCHAR(25),
    npwp_raw VARCHAR(20),
    npwp_format VARCHAR(5),
    tax_payer_type VARCHAR(20),

    -- SIM specific
    sim_number VARCHAR(20),
    sim_type VARCHAR(5),
    height VARCHAR(5),
    valid_from VARCHAR(10),
    publisher VARCHAR(100),

    -- File storage
    file_urls JSONB,

    -- Processing info
    raw_text TEXT,
    processing_time_ms BIGINT DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_ocr_records_client_id ON ocr_records(client_id);
CREATE INDEX idx_ocr_records_doc_type ON ocr_records(doc_type);
CREATE INDEX idx_ocr_records_nik ON ocr_records(nik) WHERE nik IS NOT NULL;
CREATE INDEX idx_ocr_records_npwp_raw ON ocr_records(npwp_raw) WHERE npwp_raw IS NOT NULL;
CREATE INDEX idx_ocr_records_sim_number ON ocr_records(sim_number) WHERE sim_number IS NOT NULL;
CREATE INDEX idx_ocr_records_created_at ON ocr_records(created_at);

-- Liveness Sessions (merged schema: 007 + 016 + 017 - sesuai kode terbaru)
-- id VARCHAR: liveness_service generates liv_<nik>_<timestamp>, bukan UUID
CREATE TABLE liveness_sessions (
    id VARCHAR(50) PRIMARY KEY,
    session_id VARCHAR(100) NOT NULL UNIQUE,             -- AWS Rekognition Session ID / LIV-xxx
    client_id INT NOT NULL REFERENCES clients(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,

    -- Session info (016: nik, 017: method, expires_at)
    nik VARCHAR(16),                                     -- NIK untuk sesi (AWS Rekognition)
    method VARCHAR(20) NOT NULL DEFAULT 'passive',       -- passive, active, gaze, spectrum, secure
    status liveness_status NOT NULL DEFAULT 'Pending',
    challenges JSONB NOT NULL DEFAULT '[]',              -- ["blink", "smile", "turnLeft", etc.]

    -- Result
    is_live BOOLEAN,                                     -- Final result: true/false
    confidence DECIMAL(5,2),                             -- Confidence score (0-100)
    best_frame_url VARCHAR(500),                         -- S3 URL of best face frame
    face_url TEXT,                                       -- Alias / AWS reference image URL (016)
    processing_time_ms BIGINT,                           -- Waktu pemrosesan ms (016)

    -- Face comparison (optional, if comparing with ID photo)
    reference_image_url VARCHAR(500),                    -- Reference photo (e.g. KTP photo)
    face_match BOOLEAN,                                  -- Face match result
    face_match_confidence DECIMAL(5,2),                  -- Face similarity score

    -- Provider info
    provider VARCHAR(50),                                -- 'aws_rekognition', 'internal'
    provider_session_id VARCHAR(100),                    -- Provider's session ID
    provider_data JSONB,                                 -- Raw provider response

    -- Error
    failed_reason VARCHAR(255),

    -- Timing (expires_at dari 017, expired_at untuk backward compat)
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '5 minutes'),
    expired_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '5 minutes'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT liveness_method_check CHECK (method IN ('passive', 'active', 'gaze', 'spectrum', 'secure'))
);

CREATE INDEX idx_liveness_sessions_session_id ON liveness_sessions(session_id);
CREATE INDEX idx_liveness_sessions_client_id ON liveness_sessions(client_id);
CREATE INDEX idx_liveness_sessions_status ON liveness_sessions(status);
CREATE INDEX idx_liveness_sessions_created_at ON liveness_sessions(created_at);
CREATE INDEX idx_liveness_sessions_expires_at ON liveness_sessions(expires_at);
CREATE INDEX idx_liveness_sessions_expired_at ON liveness_sessions(expired_at) WHERE status IN ('Pending', 'Processing');
CREATE INDEX idx_liveness_sessions_nik ON liveness_sessions(nik) WHERE nik IS NOT NULL;

-- Liveness frames (individual challenge frames)
CREATE TABLE liveness_frames (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL REFERENCES liveness_sessions(id) ON DELETE CASCADE,

    -- Frame info
    challenge VARCHAR(20) NOT NULL,                      -- 'BLINK', 'TURN_LEFT', 'SMILE', etc.
    frame_url VARCHAR(500) NOT NULL,                     -- S3 URL
    sequence INT NOT NULL,                               -- Frame sequence number

    -- Analysis result
    is_passed BOOLEAN,
    confidence DECIMAL(5,2),
    analysis_data JSONB,                                 -- Detailed analysis

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_liveness_frames_session_id ON liveness_frames(session_id);

-- Identity logs (unified log for OCR & Liveness provider calls)
CREATE TABLE identity_logs (
    id SERIAL PRIMARY KEY,
    ocr_request_id UUID REFERENCES ocr_requests(id) ON DELETE CASCADE,
    liveness_session_id VARCHAR(50) REFERENCES liveness_sessions(id) ON DELETE CASCADE,

    action VARCHAR(50) NOT NULL,                         -- 'ocr_extract', 'liveness_create', 'face_compare', etc.
    provider VARCHAR(50) NOT NULL,

    request JSONB,
    response JSONB,

    is_success BOOLEAN NOT NULL DEFAULT false,
    error_code VARCHAR(50),
    error_message TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    response_at TIMESTAMPTZ,
    response_time_ms INT,

    CONSTRAINT identity_logs_source_check CHECK (
        (ocr_request_id IS NOT NULL AND liveness_session_id IS NULL) OR
        (ocr_request_id IS NULL AND liveness_session_id IS NOT NULL)
    )
);

CREATE INDEX idx_identity_logs_ocr_request_id ON identity_logs(ocr_request_id);
CREATE INDEX idx_identity_logs_liveness_session_id ON identity_logs(liveness_session_id);
CREATE INDEX idx_identity_logs_action ON identity_logs(action);
CREATE INDEX idx_identity_logs_created_at ON identity_logs(created_at);
