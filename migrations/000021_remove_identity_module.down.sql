CREATE TYPE identity_doc_type AS ENUM ('KTP', 'NPWP', 'SIM');
CREATE TYPE liveness_status AS ENUM ('Pending', 'Processing', 'Passed', 'Failed', 'Expired');
CREATE TYPE ocr_status AS ENUM ('Pending', 'Processing', 'Success', 'Failed');

CREATE TABLE ocr_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ocr_id VARCHAR(50) NOT NULL UNIQUE,
    client_id INT NOT NULL REFERENCES clients(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,
    doc_type identity_doc_type NOT NULL,
    image_url TEXT,
    image_base64 TEXT,
    status ocr_status NOT NULL DEFAULT 'Pending',
    extracted_data JSONB,
    confidence_score NUMERIC(5,2),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ocr_requests_ocr_id ON ocr_requests(ocr_id);
CREATE INDEX idx_ocr_requests_client_id ON ocr_requests(client_id);
CREATE INDEX idx_ocr_requests_doc_type ON ocr_requests(doc_type);
CREATE INDEX idx_ocr_requests_status ON ocr_requests(status);
CREATE INDEX idx_ocr_requests_created_at ON ocr_requests(created_at);

CREATE TABLE ocr_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id INT NOT NULL REFERENCES clients(id),
    doc_type identity_doc_type NOT NULL,
    image_url TEXT,
    nik VARCHAR(16),
    full_name VARCHAR(255),
    birth_place VARCHAR(100),
    birth_date DATE,
    gender VARCHAR(20),
    address TEXT,
    rt_rw VARCHAR(20),
    kelurahan VARCHAR(100),
    kecamatan VARCHAR(100),
    religion VARCHAR(50),
    marital_status VARCHAR(50),
    occupation VARCHAR(100),
    nationality VARCHAR(50),
    valid_until VARCHAR(50),
    province VARCHAR(100),
    city VARCHAR(100),
    npwp_raw VARCHAR(32),
    npwp_formatted VARCHAR(32),
    registered_name VARCHAR(255),
    tax_office VARCHAR(100),
    nik_registered BOOLEAN DEFAULT false,
    npwp_registered BOOLEAN DEFAULT false,
    quality_score NUMERIC(5,2),
    confidence_score NUMERIC(5,2),
    extraction_method VARCHAR(50),
    raw_response JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sim_number VARCHAR(20),
    sim_type VARCHAR(10),
    valid_from DATE,
    valid_to DATE
);

CREATE INDEX idx_ocr_records_client_id ON ocr_records(client_id);
CREATE INDEX idx_ocr_records_doc_type ON ocr_records(doc_type);
CREATE INDEX idx_ocr_records_nik ON ocr_records(nik) WHERE nik IS NOT NULL;
CREATE INDEX idx_ocr_records_npwp_raw ON ocr_records(npwp_raw) WHERE npwp_raw IS NOT NULL;
CREATE INDEX idx_ocr_records_sim_number ON ocr_records(sim_number) WHERE sim_number IS NOT NULL;
CREATE INDEX idx_ocr_records_created_at ON ocr_records(created_at);

CREATE TABLE liveness_sessions (
    id VARCHAR(50) PRIMARY KEY,
    session_id VARCHAR(100) NOT NULL UNIQUE,
    client_id INT NOT NULL REFERENCES clients(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,
    nik VARCHAR(16),
    method VARCHAR(20) NOT NULL,
    status liveness_status NOT NULL DEFAULT 'Pending',
    confidence_score NUMERIC(5,2),
    quality_score NUMERIC(5,2),
    error_code VARCHAR(50),
    error_message TEXT,
    challenge_data JSONB,
    result_data JSONB,
    provider VARCHAR(50),
    expires_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at TIMESTAMPTZ,
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

CREATE TABLE liveness_frames (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(50) NOT NULL REFERENCES liveness_sessions(id) ON DELETE CASCADE,
    frame_type VARCHAR(50) NOT NULL,
    image_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_liveness_frames_session_id ON liveness_frames(session_id);

CREATE TABLE identity_logs (
    id BIGSERIAL PRIMARY KEY,
    ocr_request_id UUID REFERENCES ocr_requests(id) ON DELETE CASCADE,
    liveness_session_id VARCHAR(50) REFERENCES liveness_sessions(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL,
    provider VARCHAR(50),
    request JSONB,
    response JSONB,
    is_success BOOLEAN NOT NULL DEFAULT false,
    response_code VARCHAR(20),
    response_message TEXT,
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

CREATE TABLE face_comparisons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id INT NOT NULL REFERENCES clients(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,
    reference_id VARCHAR(100),
    self_image_url TEXT,
    idcard_image_url TEXT,
    similarity NUMERIC(5,2),
    threshold NUMERIC(5,2),
    status VARCHAR(20) NOT NULL DEFAULT 'Processing',
    error_code VARCHAR(50),
    error_message TEXT,
    raw_response JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_face_comparisons_client_id ON face_comparisons(client_id);
CREATE INDEX idx_face_comparisons_reference_id ON face_comparisons(reference_id);
CREATE INDEX idx_face_comparisons_status ON face_comparisons(status);
CREATE INDEX idx_face_comparisons_created_at ON face_comparisons(created_at);
