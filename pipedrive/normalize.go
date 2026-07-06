package pipedrive

// NormalizedDeal is the stable MCP response shape, independent of whether the
// raw payload came from v1 or v2. Fields we can't resolve are left zero.
type NormalizedDeal struct {
	ID             int64          `json:"id"`
	Title          string         `json:"title"`
	Status         string         `json:"status"`
	Value          float64        `json:"value,omitempty"`
	Currency       string         `json:"currency,omitempty"`
	OwnerID        int64          `json:"owner_id,omitempty"`
	PersonID       int64          `json:"person_id,omitempty"`
	OrganizationID int64          `json:"organization_id,omitempty"`
	PipelineID     int64          `json:"pipeline_id,omitempty"`
	StageID        int64          `json:"stage_id,omitempty"`
	LabelIDs       []int64        `json:"label_ids,omitempty"`
	IsArchived     bool           `json:"is_archived,omitempty"`
	AddTime        string         `json:"add_time,omitempty"`
	UpdateTime     string         `json:"update_time,omitempty"`
	CustomFields   map[string]any `json:"custom_fields,omitempty"`
}

// NormalizeDeal accepts a raw deal from either v1 or v2 and returns the stable
// shape. In v2, related IDs sit at the root and custom fields are grouped in a
// `custom_fields` object. In v1, related IDs may be nested inside *_id.value
// or expanded objects, and custom fields are top-level hash keys — we lift
// them into a `custom_fields` map here.
func NormalizeDeal(raw map[string]any) NormalizedDeal {
	d := NormalizedDeal{
		ID:             toInt64(raw["id"]),
		Title:          toString(raw["title"]),
		Status:         toString(raw["status"]),
		Value:          toFloat(raw["value"]),
		Currency:       toString(raw["currency"]),
		OwnerID:        relatedID(raw, "user_id", "owner_id"),
		PersonID:       relatedID(raw, "person_id"),
		OrganizationID: relatedID(raw, "org_id", "organization_id"),
		PipelineID:     relatedID(raw, "pipeline_id"),
		StageID:        relatedID(raw, "stage_id"),
		AddTime:        toString(raw["add_time"]),
		UpdateTime:     toString(raw["update_time"]),
	}

	// v2 groups custom fields in an object. Prefer it when present.
	if cf, ok := raw["custom_fields"].(map[string]any); ok {
		d.CustomFields = cf
	}

	if arr, ok := raw["label_ids"].([]any); ok {
		for _, v := range arr {
			if id := toInt64(v); id != 0 {
				d.LabelIDs = append(d.LabelIDs, id)
			}
		}
	}
	d.IsArchived = toBool(raw["is_archived"])

	return d
}

// NormalizeDealList walks an array payload and normalizes each element.
func NormalizeDealList(raw []any) []NormalizedDeal {
	out := make([]NormalizedDeal, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeDeal(m))
		}
	}
	return out
}

func relatedID(raw map[string]any, keys ...string) int64 {
	for _, k := range keys {
		v, ok := raw[k]
		if !ok {
			continue
		}
		// v2 returns a scalar; v1 may return {"value": n, "name": "..."}.
		if n, ok := v.(map[string]any); ok {
			if id := toInt64(n["value"]); id != 0 {
				return id
			}
			continue
		}
		if id := toInt64(v); id != 0 {
			return id
		}
	}
	return 0
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	}
	return 0
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	return 0
}

func toBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	case int64:
		return t != 0
	case int:
		return t != 0
	}
	return false
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Contact captures Pipedrive's {value,label,primary} shape for emails/phones.
type Contact struct {
	Value   string `json:"value"`
	Label   string `json:"label,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

func normalizeContacts(v any) []Contact {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]Contact, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := Contact{
			Value:   toString(m["value"]),
			Label:   toString(m["label"]),
			Primary: toBool(m["primary"]),
		}
		if c.Value != "" {
			out = append(out, c)
		}
	}
	return out
}

// NormalizedPerson is the stable shape for a Pipedrive person.
type NormalizedPerson struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	FirstName      string         `json:"first_name,omitempty"`
	LastName       string         `json:"last_name,omitempty"`
	Emails         []Contact      `json:"emails,omitempty"`
	Phones         []Contact      `json:"phones,omitempty"`
	OwnerID        int64          `json:"owner_id,omitempty"`
	OrganizationID int64          `json:"organization_id,omitempty"`
	AddTime        string         `json:"add_time,omitempty"`
	UpdateTime     string         `json:"update_time,omitempty"`
	CustomFields   map[string]any `json:"custom_fields,omitempty"`
}

func NormalizePerson(raw map[string]any) NormalizedPerson {
	p := NormalizedPerson{
		ID:             toInt64(raw["id"]),
		Name:           toString(raw["name"]),
		FirstName:      toString(raw["first_name"]),
		LastName:       toString(raw["last_name"]),
		Emails:         normalizeContacts(firstNonNil(raw, "emails", "email")),
		Phones:         normalizeContacts(firstNonNil(raw, "phones", "phone")),
		OwnerID:        relatedID(raw, "owner_id", "user_id"),
		OrganizationID: relatedID(raw, "org_id", "organization_id"),
		AddTime:        toString(raw["add_time"]),
		UpdateTime:     toString(raw["update_time"]),
	}
	if cf, ok := raw["custom_fields"].(map[string]any); ok {
		p.CustomFields = cf
	}
	return p
}

func NormalizePersonList(raw []any) []NormalizedPerson {
	out := make([]NormalizedPerson, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizePerson(m))
		}
	}
	return out
}

// NormalizedOrganization is the stable shape for a Pipedrive organization.
type NormalizedOrganization struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	OwnerID      int64          `json:"owner_id,omitempty"`
	Address      string         `json:"address,omitempty"`
	AddTime      string         `json:"add_time,omitempty"`
	UpdateTime   string         `json:"update_time,omitempty"`
	CustomFields map[string]any `json:"custom_fields,omitempty"`
}

func NormalizeOrganization(raw map[string]any) NormalizedOrganization {
	// Address in v1 is a scalar; in v2 it's {value,...}.
	address := ""
	switch t := raw["address"].(type) {
	case string:
		address = t
	case map[string]any:
		address = toString(t["value"])
	}
	o := NormalizedOrganization{
		ID:         toInt64(raw["id"]),
		Name:       toString(raw["name"]),
		OwnerID:    relatedID(raw, "owner_id", "user_id"),
		Address:    address,
		AddTime:    toString(raw["add_time"]),
		UpdateTime: toString(raw["update_time"]),
	}
	if cf, ok := raw["custom_fields"].(map[string]any); ok {
		o.CustomFields = cf
	}
	return o
}

func NormalizeOrganizationList(raw []any) []NormalizedOrganization {
	out := make([]NormalizedOrganization, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeOrganization(m))
		}
	}
	return out
}

// NormalizedActivity is the stable shape for a Pipedrive activity.
type NormalizedActivity struct {
	ID             int64  `json:"id"`
	Subject        string `json:"subject"`
	Type           string `json:"type,omitempty"`
	Done           bool   `json:"done"`
	DueDate        string `json:"due_date,omitempty"`
	DueTime        string `json:"due_time,omitempty"`
	Duration       string `json:"duration,omitempty"`
	OwnerID        int64  `json:"owner_id,omitempty"`
	DealID         int64  `json:"deal_id,omitempty"`
	PersonID       int64  `json:"person_id,omitempty"`
	OrganizationID int64  `json:"organization_id,omitempty"`
	AddTime        string `json:"add_time,omitempty"`
	UpdateTime     string `json:"update_time,omitempty"`
}

func NormalizeActivity(raw map[string]any) NormalizedActivity {
	return NormalizedActivity{
		ID:             toInt64(raw["id"]),
		Subject:        toString(raw["subject"]),
		Type:           toString(raw["type"]),
		Done:           toBool(raw["done"]),
		DueDate:        toString(raw["due_date"]),
		DueTime:        toString(raw["due_time"]),
		Duration:       toString(raw["duration"]),
		OwnerID:        relatedID(raw, "owner_id", "user_id"),
		DealID:         relatedID(raw, "deal_id"),
		PersonID:       relatedID(raw, "person_id"),
		OrganizationID: relatedID(raw, "org_id", "organization_id"),
		AddTime:        toString(raw["add_time"]),
		UpdateTime:     toString(raw["update_time"]),
	}
}

func NormalizeActivityList(raw []any) []NormalizedActivity {
	out := make([]NormalizedActivity, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeActivity(m))
		}
	}
	return out
}

// NormalizedNote is the stable shape for a Pipedrive note.
type NormalizedNote struct {
	ID             int64  `json:"id"`
	Content        string `json:"content"`
	DealID         int64  `json:"deal_id,omitempty"`
	PersonID       int64  `json:"person_id,omitempty"`
	OrganizationID int64  `json:"organization_id,omitempty"`
	UserID         int64  `json:"user_id,omitempty"`
	AddTime        string `json:"add_time,omitempty"`
	UpdateTime     string `json:"update_time,omitempty"`
}

func NormalizeNote(raw map[string]any) NormalizedNote {
	return NormalizedNote{
		ID:             toInt64(raw["id"]),
		Content:        toString(raw["content"]),
		DealID:         relatedID(raw, "deal_id"),
		PersonID:       relatedID(raw, "person_id"),
		OrganizationID: relatedID(raw, "org_id", "organization_id"),
		UserID:         relatedID(raw, "user_id"),
		AddTime:        toString(raw["add_time"]),
		UpdateTime:     toString(raw["update_time"]),
	}
}

func NormalizeNoteList(raw []any) []NormalizedNote {
	out := make([]NormalizedNote, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeNote(m))
		}
	}
	return out
}

// NormalizedFilter is the slim shape for a Pipedrive saved filter. The full
// `conditions` tree is intentionally dropped — the LLM only needs id+name+type
// to apply a filter via filter_id. Pass include_raw to see conditions.
type NormalizedFilter struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Active     bool   `json:"active"`
	VisibleTo  string `json:"visible_to,omitempty"`
	AddTime    string `json:"add_time,omitempty"`
	UpdateTime string `json:"update_time,omitempty"`
}

func NormalizeFilter(raw map[string]any) NormalizedFilter {
	return NormalizedFilter{
		ID:         toInt64(raw["id"]),
		Name:       toString(raw["name"]),
		Type:       toString(raw["type"]),
		Active:     toBool(firstNonNil(raw, "active_flag", "active")),
		VisibleTo:  toString(raw["visible_to"]),
		AddTime:    toString(raw["add_time"]),
		UpdateTime: toString(raw["update_time"]),
	}
}

func NormalizeFilterList(raw []any) []NormalizedFilter {
	out := make([]NormalizedFilter, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeFilter(m))
		}
	}
	return out
}

// ProductPrice is Pipedrive's per-currency price block.
type ProductPrice struct {
	Currency     string  `json:"currency"`
	Price        float64 `json:"price"`
	CostPrice    float64 `json:"cost_price,omitempty"`
	DirectCost   float64 `json:"direct_cost,omitempty"`
	OverheadCost float64 `json:"overhead_cost,omitempty"`
}

// NormalizedProduct is the stable shape for a Pipedrive product.
type NormalizedProduct struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Code         string         `json:"code,omitempty"`
	Unit         string         `json:"unit,omitempty"`
	Active       bool           `json:"active"`
	OwnerID      int64          `json:"owner_id,omitempty"`
	Prices       []ProductPrice `json:"prices,omitempty"`
	AddTime      string         `json:"add_time,omitempty"`
	UpdateTime   string         `json:"update_time,omitempty"`
	CustomFields map[string]any `json:"custom_fields,omitempty"`
}

func normalizePrices(v any) []ProductPrice {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]ProductPrice, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		p := ProductPrice{
			Currency:     toString(m["currency"]),
			Price:        toFloat(m["price"]),
			CostPrice:    toFloat(m["cost_price"]),
			DirectCost:   toFloat(m["direct_cost"]),
			OverheadCost: toFloat(m["overhead_cost"]),
		}
		if p.Currency != "" {
			out = append(out, p)
		}
	}
	return out
}

func NormalizeProduct(raw map[string]any) NormalizedProduct {
	p := NormalizedProduct{
		ID:         toInt64(raw["id"]),
		Name:       toString(raw["name"]),
		Code:       toString(raw["code"]),
		Unit:       toString(raw["unit"]),
		Active:     toBool(firstNonNil(raw, "active_flag", "active")),
		OwnerID:    relatedID(raw, "owner_id", "user_id"),
		Prices:     normalizePrices(raw["prices"]),
		AddTime:    toString(raw["add_time"]),
		UpdateTime: toString(raw["update_time"]),
	}
	if cf, ok := raw["custom_fields"].(map[string]any); ok {
		p.CustomFields = cf
	}
	return p
}

func NormalizeProductList(raw []any) []NormalizedProduct {
	out := make([]NormalizedProduct, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeProduct(m))
		}
	}
	return out
}

// NormalizedFollower captures the minimal follower shape.
type NormalizedFollower struct {
	ID      int64  `json:"id"`
	UserID  int64  `json:"user_id"`
	AddTime string `json:"add_time,omitempty"`
}

func NormalizeFollower(raw map[string]any) NormalizedFollower {
	return NormalizedFollower{
		ID:      toInt64(raw["id"]),
		UserID:  relatedID(raw, "user_id"),
		AddTime: toString(raw["add_time"]),
	}
}

func NormalizeFollowerList(raw []any) []NormalizedFollower {
	out := make([]NormalizedFollower, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeFollower(m))
		}
	}
	return out
}

// NormalizedDealProduct represents a product attached to a deal.
type NormalizedDealProduct struct {
	ID          int64   `json:"id"`
	DealID      int64   `json:"deal_id"`
	ProductID   int64   `json:"product_id"`
	Name        string  `json:"name,omitempty"`
	Quantity    float64 `json:"quantity"`
	ItemPrice   float64 `json:"item_price,omitempty"`
	Discount    float64 `json:"discount,omitempty"`
	DiscountType string `json:"discount_type,omitempty"`
	Tax         float64 `json:"tax,omitempty"`
	Currency    string  `json:"currency,omitempty"`
	Comments    string  `json:"comments,omitempty"`
	AddTime     string  `json:"add_time,omitempty"`
}

func NormalizeDealProduct(raw map[string]any) NormalizedDealProduct {
	return NormalizedDealProduct{
		ID:           toInt64(raw["id"]),
		DealID:       relatedID(raw, "deal_id"),
		ProductID:    relatedID(raw, "product_id"),
		Name:         toString(raw["name"]),
		Quantity:     toFloat(raw["quantity"]),
		ItemPrice:    toFloat(raw["item_price"]),
		Discount:     toFloat(raw["discount"]),
		DiscountType: toString(raw["discount_type"]),
		Tax:          toFloat(raw["tax"]),
		Currency:     toString(raw["currency"]),
		Comments:     toString(raw["comments"]),
		AddTime:      toString(raw["add_time"]),
	}
}

func NormalizeDealProductList(raw []any) []NormalizedDealProduct {
	out := make([]NormalizedDealProduct, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeDealProduct(m))
		}
	}
	return out
}

// NormalizedLead is the stable shape for a Pipedrive lead (v1 only).
type NormalizedLead struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	OwnerID        int64          `json:"owner_id,omitempty"`
	PersonID       int64          `json:"person_id,omitempty"`
	OrganizationID int64          `json:"organization_id,omitempty"`
	Value          float64        `json:"value,omitempty"`
	Currency       string         `json:"currency,omitempty"`
	ExpectedCloseDate string      `json:"expected_close_date,omitempty"`
	LabelIDs       []string       `json:"label_ids,omitempty"`
	AddTime        string         `json:"add_time,omitempty"`
	UpdateTime     string         `json:"update_time,omitempty"`
	CustomFields   map[string]any `json:"custom_fields,omitempty"`
}

func NormalizeLead(raw map[string]any) NormalizedLead {
	l := NormalizedLead{
		ID:                toString(raw["id"]),
		Title:             toString(raw["title"]),
		OwnerID:           relatedID(raw, "owner_id", "user_id"),
		PersonID:          relatedID(raw, "person_id"),
		OrganizationID:    relatedID(raw, "organization_id", "org_id"),
		ExpectedCloseDate: toString(raw["expected_close_date"]),
		AddTime:           toString(raw["add_time"]),
		UpdateTime:        toString(raw["update_time"]),
	}
	// Lead value is an object {amount, currency} in v1.
	if v, ok := raw["value"].(map[string]any); ok {
		l.Value = toFloat(v["amount"])
		l.Currency = toString(v["currency"])
	}
	if arr, ok := raw["label_ids"].([]any); ok {
		for _, id := range arr {
			if s, ok := id.(string); ok {
				l.LabelIDs = append(l.LabelIDs, s)
			}
		}
	}
	if cf, ok := raw["custom_fields"].(map[string]any); ok {
		l.CustomFields = cf
	}
	return l
}

func NormalizeLeadList(raw []any) []NormalizedLead {
	out := make([]NormalizedLead, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeLead(m))
		}
	}
	return out
}

func firstNonNil(raw map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := raw[k]; ok && v != nil {
			return v
		}
	}
	return nil
}
