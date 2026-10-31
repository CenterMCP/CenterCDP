package cdp

const registrySchema = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS customer_databases (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    database_path TEXT NOT NULL UNIQUE,
    schema_version INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const databaseSchema = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS contact_attribute_definitions (
    id TEXT PRIMARY KEY,
    attribute_key TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL,
    data_type TEXT NOT NULL CHECK (data_type IN ('string','integer','number','boolean','date','datetime','enum','object','array')),
    canonical_field TEXT,
    is_required INTEGER NOT NULL DEFAULT 0,
    is_filterable INTEGER NOT NULL DEFAULT 1,
    is_searchable INTEGER NOT NULL DEFAULT 0,
    is_pii INTEGER NOT NULL DEFAULT 0,
    enum_values_json TEXT,
    validation_json TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS event_definitions (
    id TEXT PRIMARY KEY,
    event_key TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL,
    canonical_event TEXT,
    allow_extra_parameters INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS event_parameter_definitions (
    id TEXT PRIMARY KEY,
    event_definition_id TEXT NOT NULL,
    parameter_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL,
    data_type TEXT NOT NULL CHECK (data_type IN ('string','integer','number','boolean','date','datetime','enum','object','array')),
    canonical_field TEXT,
    is_required INTEGER NOT NULL DEFAULT 0,
    is_filterable INTEGER NOT NULL DEFAULT 1,
    is_pii INTEGER NOT NULL DEFAULT 0,
    enum_values_json TEXT,
    validation_json TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(event_definition_id, parameter_key),
    FOREIGN KEY(event_definition_id) REFERENCES event_definitions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS contacts (
    id TEXT PRIMARY KEY,
    attributes_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS contact_identities (
    id TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL,
    identity_type TEXT NOT NULL,
    normalized_value TEXT NOT NULL,
    original_value TEXT NOT NULL,
    is_primary INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE(identity_type, normalized_value),
    FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_contact_identities_contact ON contact_identities(contact_id);

CREATE TABLE IF NOT EXISTS contact_consents (
    contact_id TEXT NOT NULL,
    consent_key TEXT NOT NULL,
    granted INTEGER NOT NULL,
    source TEXT,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(contact_id, consent_key),
    FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    contact_id TEXT,
    event_key TEXT NOT NULL,
    definition_version INTEGER NOT NULL,
    occurred_at TEXT NOT NULL,
    properties_json TEXT NOT NULL DEFAULT '{}',
    idempotency_key TEXT UNIQUE,
    created_at TEXT NOT NULL,
    FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_events_contact_key_time ON events(contact_id, event_key, occurred_at);

CREATE TABLE IF NOT EXISTS segments (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    definition_type TEXT NOT NULL CHECK (definition_type IN ('rule','sql')),
    definition_json TEXT,
    sql_query TEXT,
    sql_params_json TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','validated','archived')),
    last_validated_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS segment_memberships (
    segment_id TEXT NOT NULL,
    segment_version INTEGER NOT NULL,
    contact_id TEXT NOT NULL,
    evaluated_at TEXT NOT NULL,
    PRIMARY KEY(segment_id, segment_version, contact_id),
    FOREIGN KEY(segment_id) REFERENCES segments(id) ON DELETE CASCADE,
    FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('email','sms','push')),
    subject TEXT,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','archived')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS campaigns (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    segment_id TEXT NOT NULL,
    template_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','ready','running','paused','completed')),
    created_at TEXT NOT NULL,
    launched_at TEXT,
    FOREIGN KEY(segment_id) REFERENCES segments(id),
    FOREIGN KEY(template_id) REFERENCES message_templates(id)
);

CREATE TABLE IF NOT EXISTS campaign_audience (
    campaign_id TEXT NOT NULL,
    contact_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL,
    PRIMARY KEY(campaign_id, contact_id),
    FOREIGN KEY(campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS journeys (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','paused','archived')),
    timezone TEXT NOT NULL DEFAULT 'UTC',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS journey_versions (
    id TEXT PRIMARY KEY,
    journey_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','retired')),
    published_at TEXT,
    created_at TEXT NOT NULL,
    UNIQUE(journey_id, version),
    FOREIGN KEY(journey_id) REFERENCES journeys(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS journey_steps (
    id TEXT PRIMARY KEY,
    journey_version_id TEXT NOT NULL,
    step_key TEXT NOT NULL,
    step_type TEXT NOT NULL CHECK (step_type IN ('trigger','action','delay','condition','end')),
    name TEXT NOT NULL,
    configuration_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE(journey_version_id, step_key),
    FOREIGN KEY(journey_version_id) REFERENCES journey_versions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS journey_transitions (
    id TEXT PRIMARY KEY,
    journey_version_id TEXT NOT NULL,
    from_step_id TEXT NOT NULL,
    outcome TEXT NOT NULL,
    to_step_id TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    UNIQUE(from_step_id, outcome),
    FOREIGN KEY(journey_version_id) REFERENCES journey_versions(id) ON DELETE CASCADE,
    FOREIGN KEY(from_step_id) REFERENCES journey_steps(id) ON DELETE CASCADE,
    FOREIGN KEY(to_step_id) REFERENCES journey_steps(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS journey_enrollments (
    id TEXT PRIMARY KEY,
    journey_version_id TEXT NOT NULL,
    contact_id TEXT NOT NULL,
    trigger_event_id TEXT,
    enrollment_key TEXT NOT NULL UNIQUE,
    current_step_id TEXT,
    status TEXT NOT NULL CHECK (status IN ('active','waiting','completed','cancelled','failed')),
    enrolled_at TEXT NOT NULL,
    next_run_at TEXT,
    completed_at TEXT,
    context_json TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY(journey_version_id) REFERENCES journey_versions(id),
    FOREIGN KEY(contact_id) REFERENCES contacts(id),
    FOREIGN KEY(current_step_id) REFERENCES journey_steps(id)
);
CREATE INDEX IF NOT EXISTS idx_journey_enrollments_due ON journey_enrollments(status, next_run_at);

CREATE TABLE IF NOT EXISTS journey_step_runs (
    id TEXT PRIMARY KEY,
    enrollment_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL CHECK (status IN ('pending','processing','completed','failed','cancelled')),
    idempotency_key TEXT NOT NULL UNIQUE,
    scheduled_at TEXT,
    started_at TEXT,
    completed_at TEXT,
    result_json TEXT,
    error_message TEXT,
    FOREIGN KEY(enrollment_id) REFERENCES journey_enrollments(id) ON DELETE CASCADE,
    FOREIGN KEY(step_id) REFERENCES journey_steps(id)
);

CREATE TABLE IF NOT EXISTS message_outbox (
    id TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL,
    journey_step_run_id TEXT,
    campaign_id TEXT,
    channel TEXT NOT NULL,
    template_id TEXT NOT NULL,
    recipient TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    idempotency_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sending','sent','failed','suppressed')),
    scheduled_at TEXT NOT NULL,
    provider_message_id TEXT,
    attempt INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TEXT NOT NULL,
    sent_at TEXT,
    FOREIGN KEY(contact_id) REFERENCES contacts(id),
    FOREIGN KEY(journey_step_run_id) REFERENCES journey_step_runs(id),
    FOREIGN KEY(campaign_id) REFERENCES campaigns(id),
    FOREIGN KEY(template_id) REFERENCES message_templates(id)
);
CREATE INDEX IF NOT EXISTS idx_message_outbox_pending ON message_outbox(status, scheduled_at);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT,
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
`
