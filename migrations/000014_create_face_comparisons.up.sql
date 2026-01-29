-- ============================================
-- Migration: Create Face Comparisons Table
-- ============================================

CREATE TABLE IF NOT EXISTS face_comparisons (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id INT NOT NULL REFERENCES clients(id),

    -- Input
    source_type VARCHAR(20) NOT NULL, -- 'url' or 'upload'
    source_url TEXT NOT NULL, -- S3 URL or uploaded file path
    target_type VARCHAR(20) NOT NULL, -- 'url' or 'upload'
    target_url TEXT NOT NULL, -- S3 URL or uploaded file path

    -- Result
    matched BOOLEAN NOT NULL,
    similarity DECIMAL(5,2) NOT NULL, -- 0.00 - 100.00
    threshold DECIMAL(5,2), -- Optional: similarity threshold sent to AWS (null = AWS default)

    -- Face 1 Detection
    source_detected BOOLEAN NOT NULL DEFAULT true,
    source_confidence DECIMAL(5,2),
    source_bounding_box JSONB,

    -- Face 2 Detection
    target_detected BOOLEAN NOT NULL DEFAULT true,
    target_confidence DECIMAL(5,2),
    target_bounding_box JSONB,

    -- Metadata
    processing_time_ms INTEGER,
    aws_request_id VARCHAR(100),
    error_code VARCHAR(50),
    error_message TEXT,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Constraints
    CONSTRAINT face_comparisons_similarity_range CHECK (similarity >= 0 AND similarity <= 100),
    CONSTRAINT face_comparisons_threshold_range CHECK (threshold IS NULL OR (threshold >= 0 AND threshold <= 100)),
    CONSTRAINT face_comparisons_source_type_check CHECK (source_type IN ('url', 'upload')),
    CONSTRAINT face_comparisons_target_type_check CHECK (target_type IN ('url', 'upload'))
);

-- Indexes
CREATE INDEX idx_face_comparisons_client_id ON face_comparisons(client_id);
CREATE INDEX idx_face_comparisons_matched ON face_comparisons(matched);
CREATE INDEX idx_face_comparisons_created_at ON face_comparisons(created_at);

-- Comments
COMMENT ON TABLE face_comparisons IS 'Face comparison results from AWS Rekognition CompareFaces';
COMMENT ON COLUMN face_comparisons.source_type IS 'Input type: url (S3) or upload (multipart)';
COMMENT ON COLUMN face_comparisons.similarity IS 'Similarity percentage from AWS (0-100)';
COMMENT ON COLUMN face_comparisons.threshold IS 'Optional: similarity threshold sent to AWS (null = AWS default)';
COMMENT ON COLUMN face_comparisons.source_bounding_box IS 'JSON: {width, height, left, top}';
