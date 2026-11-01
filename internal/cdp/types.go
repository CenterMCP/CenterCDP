package cdp

type DefineContactAttributeInput struct {
	DatabaseID     string   `json:"database_id" jsonschema:"required"`
	AttributeKey   string   `json:"attribute_key" jsonschema:"required"`
	DisplayName    string   `json:"display_name" jsonschema:"required"`
	Description    string   `json:"description" jsonschema:"required"`
	DataType       string   `json:"data_type" jsonschema:"required"`
	CanonicalField string   `json:"canonical_field,omitempty"`
	Required       bool     `json:"required,omitempty"`
	Filterable     bool     `json:"filterable,omitempty"`
	Searchable     bool     `json:"searchable,omitempty"`
	PII            bool     `json:"pii,omitempty"`
	EnumValues     []string `json:"enum_values,omitempty"`
}

type EventParameterInput struct {
	ParameterKey   string   `json:"parameter_key" jsonschema:"required"`
	DisplayName    string   `json:"display_name" jsonschema:"required"`
	Description    string   `json:"description" jsonschema:"required"`
	DataType       string   `json:"data_type" jsonschema:"required"`
	CanonicalField string   `json:"canonical_field,omitempty"`
	Required       bool     `json:"required,omitempty"`
	Filterable     bool     `json:"filterable,omitempty"`
	PII            bool     `json:"pii,omitempty"`
	EnumValues     []string `json:"enum_values,omitempty"`
}

type DefineEventInput struct {
	DatabaseID           string                `json:"database_id" jsonschema:"required"`
	EventKey             string                `json:"event_key" jsonschema:"required"`
	DisplayName          string                `json:"display_name" jsonschema:"required"`
	Description          string                `json:"description" jsonschema:"required"`
	CanonicalEvent       string                `json:"canonical_event,omitempty"`
	AllowExtraParameters bool                  `json:"allow_extra_parameters,omitempty"`
	Parameters           []EventParameterInput `json:"parameters,omitempty"`
}

type IdentityInput struct {
	Type      string `json:"type" jsonschema:"required"`
	Value     string `json:"value" jsonschema:"required"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

type UpsertContactInput struct {
	DatabaseID    string          `json:"database_id" jsonschema:"required"`
	ContactID     string          `json:"contact_id,omitempty"`
	Identities    []IdentityInput `json:"identities" jsonschema:"required"`
	Attributes    map[string]any  `json:"attributes,omitempty"`
	Consents      map[string]bool `json:"consents,omitempty"`
	ConsentSource string          `json:"consent_source,omitempty"`
}

type ContactResult struct {
	DatabaseID string         `json:"database_id"`
	ContactID  string         `json:"contact_id"`
	Created    bool           `json:"created"`
	Attributes map[string]any `json:"attributes"`
}

type GetContactInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	ContactID  string `json:"contact_id" jsonschema:"required"`
}

type SearchContactsInput struct {
	DatabaseID   string `json:"database_id" jsonschema:"required"`
	AttributeKey string `json:"attribute_key" jsonschema:"required"`
	Value        any    `json:"value" jsonschema:"required"`
	Limit        int    `json:"limit,omitempty"`
}

type PageInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
}

type StatusPageInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	Status     string `json:"status,omitempty"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
}

type PageResult struct {
	Rows       []map[string]any `json:"rows"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	Total      int              `json:"total"`
	TotalPages int              `json:"total_pages"`
}

type TrackEventInput struct {
	DatabaseID     string         `json:"database_id" jsonschema:"required"`
	ContactID      string         `json:"contact_id,omitempty"`
	EventKey       string         `json:"event_key" jsonschema:"required"`
	OccurredAt     string         `json:"occurred_at,omitempty"`
	Properties     map[string]any `json:"properties,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

type CreateSegmentInput struct {
	DatabaseID  string         `json:"database_id" jsonschema:"required"`
	Name        string         `json:"name" jsonschema:"required"`
	Description string         `json:"description,omitempty"`
	Type        string         `json:"type" jsonschema:"required"`
	Rule        map[string]any `json:"rule,omitempty"`
	SQL         string         `json:"sql,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type PreviewSegmentInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	SegmentID  string `json:"segment_id" jsonschema:"required"`
}

type SegmentTargetInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	SegmentID  string `json:"segment_id" jsonschema:"required"`
}

type MultiDatabaseSegmentPreviewInput struct {
	Targets []SegmentTargetInput `json:"targets" jsonschema:"required"`
}

type SegmentPreview struct {
	DatabaseID       string   `json:"database_id"`
	SegmentID        string   `json:"segment_id"`
	Count            int      `json:"count"`
	SampleContactIDs []string `json:"sample_contact_ids"`
	DurationMS       int64    `json:"duration_ms"`
}

type SaveTemplateInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	Name       string `json:"name" jsonschema:"required"`
	Channel    string `json:"channel" jsonschema:"required"`
	Subject    string `json:"subject,omitempty"`
	Body       string `json:"body" jsonschema:"required"`
	Active     bool   `json:"active,omitempty"`
}

type CreateCampaignInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	Name       string `json:"name" jsonschema:"required"`
	SegmentID  string `json:"segment_id" jsonschema:"required"`
	TemplateID string `json:"template_id" jsonschema:"required"`
}

type CampaignInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	CampaignID string `json:"campaign_id" jsonschema:"required"`
}

type JourneyStepInput struct {
	Key           string         `json:"key" jsonschema:"required"`
	Type          string         `json:"type" jsonschema:"required"`
	Name          string         `json:"name" jsonschema:"required"`
	Configuration map[string]any `json:"configuration,omitempty"`
}

type JourneyTransitionInput struct {
	From     string `json:"from" jsonschema:"required"`
	Outcome  string `json:"outcome" jsonschema:"required"`
	To       string `json:"to" jsonschema:"required"`
	Priority int    `json:"priority,omitempty"`
}

type CreateJourneyInput struct {
	DatabaseID  string                   `json:"database_id" jsonschema:"required"`
	Name        string                   `json:"name" jsonschema:"required"`
	Description string                   `json:"description,omitempty"`
	Timezone    string                   `json:"timezone,omitempty"`
	Steps       []JourneyStepInput       `json:"steps" jsonschema:"required"`
	Transitions []JourneyTransitionInput `json:"transitions" jsonschema:"required"`
}

type JourneyInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	JourneyID  string `json:"journey_id" jsonschema:"required"`
}

type DatabaseInput struct {
	Name string `json:"name" jsonschema:"required"`
}

type DatabaseScopedInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
}

type OutboxListInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	Status     string `json:"status,omitempty"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
	// Limit is kept for backwards compatibility. Prefer page_size.
	Limit int `json:"limit,omitempty"`
}
