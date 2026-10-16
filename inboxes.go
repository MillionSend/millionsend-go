package millionsend

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Inbox is an address whose mail lands in threads, as returned by Create,
// Get and Update, and as a List row (rows carry no Object, DomainId,
// ReceivingAddress, Drafts, CreatedAt, Quarantined or PendingApproval).
type Inbox struct {
	Object string `json:"object"`
	Id     string `json:"id"`
	Name   string `json:"name"`
	// EmailAddress is lowercase and fixed at creation.
	EmailAddress string `json:"email_address"`
	// DomainId is the custom domain, or the team's subdomain of the shared
	// inbox domain: the Domains.List row with Shared true.
	DomainId         string `json:"domain_id"`
	ReceivingAddress string `json:"receiving_address"` // set for forwarding inboxes
	FromName         string `json:"from_name"`
	Unread           int    `json:"unread"` // threads with unread mail
	Drafts           int    `json:"drafts"` // unsent drafts
	LastReceived     string `json:"last_received"`
	CreatedAt        string `json:"created_at"`

	Kind            string `json:"kind"` // "agent" | "human"
	Shared          bool   `json:"shared"`
	ExpiresAt       string `json:"expires_at"`  // "" when it never expires
	Quarantined     int    `json:"quarantined"` // held messages not yet released
	PendingApproval int    `json:"pending_approval"`
}

// CreateInboxRequest is the payload for Inboxes.Create; every field is
// optional. Without EmailAddress the inbox gets an address on the shared inbox
// domain, Username@<team>.<shared domain>, random without a Username. There
// Username is trimmed and lowercased: 1-64 of a-z 0-9 . _ -, starting and
// ending with a letter or digit, without ".."; reserved names (postmaster,
// abuse, admin, …), a "fwd-" prefix and brand look-alikes get 422, and one in
// use 409. DomainId, instead of EmailAddress, is the id of a Domains.List
// row: the shared row works like leaving EmailAddress out, and one of the
// team's own domains needs Username, the part before the @ under
// EmailAddress's rules, giving Username@<domain> (receiving on, or
// Forwarding). Name takes 1-320 characters. Kind is "agent" (the default:
// screening, code extraction and send rules on) or "human". ExpiresIn runs
// from "1h" to "365d"; empty, the inbox never expires. Safety sets any of the
// rules at creation.
type CreateInboxRequest struct {
	EmailAddress string             `json:"email_address,omitempty"`
	DomainId     string             `json:"domain_id,omitempty"`
	Username     string             `json:"username,omitempty"`
	Name         string             `json:"name,omitempty"`
	FromName     string             `json:"from_name,omitempty"`
	Forwarding   bool               `json:"forwarding,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	ExpiresIn    string             `json:"expires_in,omitempty"`
	Safety       *InboxSafetyUpdate `json:"safety,omitempty"`
}

// UpdateInboxRequest is the payload for Inboxes.Update; empty fields are left
// unchanged. Name takes 1-320 characters. FromName: Ptr("") removes the
// display name. ClearExpiresIn makes the inbox never expire.
type UpdateInboxRequest struct {
	Name      string  `json:"name,omitempty"`
	FromName  *string `json:"from_name,omitempty"`
	ExpiresIn string  `json:"expires_in,omitempty"`

	nulls []string
}

// ClearExpiresIn sends expires_in as null: the inbox never expires.
func (r *UpdateInboxRequest) ClearExpiresIn() {
	r.nulls = append(r.nulls, "expires_in")
}

// MarshalJSON adds the cleared fields as explicit nulls.
func (r UpdateInboxRequest) MarshalJSON() ([]byte, error) {
	type plain UpdateInboxRequest
	return marshalWithNulls(plain(r), r.nulls)
}

// ListInboxesOptions paginates Inboxes.List (newest first) and optionally
// filters by Kind.
type ListInboxesOptions struct {
	Kind   string
	Limit  int
	After  string
	Before string
}

// InboxSafety is an inbox's rules: who may write in and what an agent may send.
// On a shared inbox, SendPolicy "open" is accepted only while the shared row's
// SendingMode is "open" and "allowlist" never, OwnerAddresses don't make
// senders trusted, and ForwardTo copies go only to team members' sign-in
// addresses.
type InboxSafety struct {
	Object          string   `json:"object"`
	InboxId         string   `json:"inbox_id"`
	AcceptFrom      string   `json:"accept_from"` // "anyone" | "verified" | "allowlist"
	Allowlist       []string `json:"allowlist"`   // "local@domain" or "*@domain"
	OwnerAddresses  []string `json:"owner_addresses"`
	Screening       bool     `json:"screening"`
	ExtractCodes    bool     `json:"extract_codes"`
	SendPolicy      string   `json:"send_policy"` // "reply_only" | "allowlist" | "open"
	RequireApproval bool     `json:"require_approval"`
	PrivateCodes    bool     `json:"private_codes"`
	SendCap         *int     `json:"send_cap"` // recipients per UTC day; nil: no inbox cap
	ForwardTo       []string `json:"forward_to"`
}

// InboxSafetyUpdate is a partial set of rules for Safety.Update and
// CreateInboxRequest.Safety; nil and empty fields are left unchanged.
// ClearSendCap removes the daily cap.
type InboxSafetyUpdate struct {
	AcceptFrom      string    `json:"accept_from,omitempty"`
	Allowlist       *[]string `json:"allowlist,omitempty"`
	OwnerAddresses  *[]string `json:"owner_addresses,omitempty"`
	Screening       *bool     `json:"screening,omitempty"`
	ExtractCodes    *bool     `json:"extract_codes,omitempty"`
	SendPolicy      string    `json:"send_policy,omitempty"`
	RequireApproval *bool     `json:"require_approval,omitempty"`
	PrivateCodes    *bool     `json:"private_codes,omitempty"`
	SendCap         *int      `json:"send_cap,omitempty"`
	ForwardTo       *[]string `json:"forward_to,omitempty"`

	nulls []string
}

// ClearSendCap sends send_cap as null: no daily cap for the inbox.
func (r *InboxSafetyUpdate) ClearSendCap() {
	r.nulls = append(r.nulls, "send_cap")
}

// MarshalJSON adds the cleared fields as explicit nulls.
func (r InboxSafetyUpdate) MarshalJSON() ([]byte, error) {
	type plain InboxSafetyUpdate
	return marshalWithNulls(plain(r), r.nulls)
}

// VerificationCode is the answer of Codes.Latest. Status is "found", "none",
// "ambiguous" or "timeout"; a found Kind is "otp" (Code) or "link" (Url, never
// fetched by the server).
type VerificationCode struct {
	Object                  string `json:"object"`
	Status                  string `json:"status"`
	Kind                    string `json:"kind"`
	Code                    string `json:"code"`
	Url                     string `json:"url"`
	LinkDomainMatchesSender *bool  `json:"link_domain_matches_sender"`
	EmailId                 string `json:"email_id"`
	From                    string `json:"from"`
	ReceivedAt              string `json:"received_at"`
	SenderTrust             string `json:"sender_trust"`
}

// LatestCodeOptions filters Codes.Latest. From is an address, or a domain
// covering its subdomains. Wait (up to 50 seconds) long-polls for a new
// code. MaxAge (60-86400 seconds, server default 600) bounds how long ago the
// email reached the inbox (a released one counts from its release). Since
// counts only mail received after that instant: pass the ReceivedAt of the
// code already used, so a second sign-in waits for its own.
type LatestCodeOptions struct {
	From   string
	Wait   int
	MaxAge int
	Since  string // ISO 8601 with an offset, as time.RFC3339 writes it
}

// QuarantinedEmail is a held message in Quarantine.List.
type QuarantinedEmail struct {
	Id               string `json:"id"`
	ThreadId         string `json:"thread_id"`
	From             string `json:"from"`
	Subject          string `json:"subject"`
	ReceivedAt       string `json:"received_at"`
	QuarantineReason string `json:"quarantine_reason"`
	SenderTrust      string `json:"sender_trust"`
	InjectionScore   *int   `json:"injection_score"`
	PhishingScore    *int   `json:"phishing_score"`
	VirusSignature   string `json:"virus_signature"`
}

// ReleaseQuarantinedRequest is the payload for Quarantine.Release.
// TrustSender also adds the sender to the inbox's allowlist.
type ReleaseQuarantinedRequest struct {
	TrustSender bool `json:"trust_sender,omitempty"`
}

// QuarantineRelease is returned by Quarantine.Release. TrustedSender is the
// address added to the allowlist, "" when none was.
type QuarantineRelease struct {
	Object        string `json:"object"`
	InboxId       string `json:"inbox_id"`
	EmailId       string `json:"email_id"`
	ReleasedAt    string `json:"released_at"`
	TrustedSender string `json:"trusted_sender"`
}

// InboxLabel is a label of one inbox; on a thread it carries no Object or
// CreatedAt. Color is "#RRGGBB", uppercase.
type InboxLabel struct {
	Object    string `json:"object"`
	Id        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
}

// CreateInboxLabelRequest is the payload for Labels.Create; an empty Color
// gets a random one.
type CreateInboxLabelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// UpdateInboxLabelRequest is the payload for Labels.Update; empty fields are
// left unchanged.
type UpdateInboxLabelRequest struct {
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// InboxThread is a conversation, as returned by Threads.Get and Update.
// Quarantined counts held messages and is 0 for inbox-scoped keys.
type InboxThread struct {
	Object      string       `json:"object"`
	Id          string       `json:"id"`
	Subject     string       `json:"subject"`
	Folder      string       `json:"folder"` // "inbox" | "archive" | "spam" | "sent" | "trash"
	Labels      []InboxLabel `json:"labels"`
	Read        bool         `json:"read"`
	Quarantined int          `json:"quarantined"`
}

// InboxThreadListItem is a row of Threads.List and Threads.Search. From is
// the latest inbound sender ("Name <address>"); To, Cc and Bcc are the latest
// message's; ReceivedAt is the last activity. MatchedEmailId and Highlights
// (matched fragments marked **like this**) come with Search only. Quarantined
// counts the messages in quarantine not yet released (mail still being
// screened isn't counted); the API sends it only with IncludeQuarantined,
// so it is 0 otherwise.
type InboxThreadListItem struct {
	Id             string              `json:"id"`
	Subject        string              `json:"subject"`
	From           string              `json:"from"`
	To             []string            `json:"to"`
	Cc             []string            `json:"cc"`
	Bcc            []string            `json:"bcc"`
	Labels         []InboxLabel        `json:"labels"`
	MessageCount   int                 `json:"message_count"`
	HasAttachment  bool                `json:"has_attachment"`
	HasDraft       bool                `json:"has_draft"`
	Read           bool                `json:"read"`
	Quarantined    int                 `json:"quarantined"`
	ReceivedAt     string              `json:"received_at"`
	Folder         string              `json:"folder"`
	MatchedEmailId string              `json:"matched_email_id"`
	Highlights     map[string][]string `json:"highlights"`
}

// ListInboxThreadsOptions filters Threads.List (most recently active first).
// Folders defaults to inbox (inbox, archive and sent when Labels is set);
// Labels matches any of the label ids; IncludeQuarantined also counts held
// mail and fills each item's Quarantined (full-access keys not scoped to an
// inbox; an inbox-scoped key gets 403 restricted_api_key).
type ListInboxThreadsOptions struct {
	Folders            []string
	Labels             []string
	Read               *bool
	IncludeQuarantined bool
	Limit              int
	After              string
	Before             string
}

func (o *ListInboxThreadsOptions) values() url.Values {
	if o == nil {
		return nil
	}
	v := (&ListOptions{Limit: o.Limit, After: o.After, Before: o.Before}).values()
	setQuery(v, "folders", o.Folders)
	setQuery(v, "labels", o.Labels)
	setQuery(v, "read", o.Read)
	setQuery(v, "include_quarantined", o.IncludeQuarantined)
	return v
}

// SearchInboxThreadsOptions filters Threads.Search: the list filters plus
// Query (every word must match; "exact phrase" and -word work), From, To, Cc
// and Bcc (substrings of an address or name, any-match), HasAttachment and a
// StartDate/EndDate range (YYYY-MM-DD or ISO 8601). It searches subjects,
// senders and recipients.
type SearchInboxThreadsOptions struct {
	Query              string
	From               []string
	To                 []string
	Cc                 []string
	Bcc                []string
	HasAttachment      *bool
	StartDate          string
	EndDate            string
	Folders            []string
	Labels             []string
	Read               *bool
	IncludeQuarantined bool
	Limit              int
	After              string
	Before             string
}

func (o *SearchInboxThreadsOptions) values() url.Values {
	if o == nil {
		return nil
	}
	v := (&ListInboxThreadsOptions{
		Folders: o.Folders, Labels: o.Labels, Read: o.Read, IncludeQuarantined: o.IncludeQuarantined,
		Limit: o.Limit, After: o.After, Before: o.Before,
	}).values()
	setQuery(v, "query", o.Query)
	setQuery(v, "from", o.From)
	setQuery(v, "to", o.To)
	setQuery(v, "cc", o.Cc)
	setQuery(v, "bcc", o.Bcc)
	setQuery(v, "has_attachment", o.HasAttachment)
	setQuery(v, "start_date", o.StartDate)
	setQuery(v, "end_date", o.EndDate)
	return v
}

// UpdateInboxThreadRequest is the payload for Threads.Update: Read marks
// every message, Folder moves the thread ("inbox", "archive", "spam" or
// "trash"), LabelId applies a label.
type UpdateInboxThreadRequest struct {
	Read    *bool  `json:"read,omitempty"`
	Folder  string `json:"folder,omitempty"`
	LabelId string `json:"label_id,omitempty"`
}

// InboxThreadEmail is a message of a thread, in either direction. Inbound
// messages add ExtractedText, Screening, SpamVerdict and VirusVerdict;
// outbound ones add Status, the email's last delivery event. A Reply or
// Forward that went out also carries EmailId, ThreadId and InboxId.
type InboxThreadEmail struct {
	Id          string            `json:"id"`
	Direction   string            `json:"direction"` // "inbound" | "outbound"
	From        string            `json:"from"`      // "Name <address>" when there is a display name
	To          []string          `json:"to"`
	Cc          []string          `json:"cc"`
	Bcc         []string          `json:"bcc"`
	ReplyTo     []string          `json:"reply_to"`
	Subject     string            `json:"subject"`
	MessageId   string            `json:"message_id"`
	Html        string            `json:"html"`
	Text        string            `json:"text"` // inbound: invisible characters removed
	Attachments []EmailAttachment `json:"attachments"`
	Read        bool              `json:"read"`
	ReceivedAt  string            `json:"received_at"`

	ExtractedText string     `json:"extracted_text"`
	Screening     *Screening `json:"screening"`
	SpamVerdict   string     `json:"spam_verdict"`
	VirusVerdict  string     `json:"virus_verdict"`
	Status        string     `json:"status"`
	EmailId       string     `json:"email_id"`
	ThreadId      string     `json:"thread_id"`
	InboxId       string     `json:"inbox_id"`
}

// ReplyInThreadRequest is the payload for Threads.Emails.Reply: Text and/or
// Html, plus optional Subject (default "Re: " and the thread's), Cc, Bcc,
// ReplyAll and RequiresApproval (hold it as a draft for a person).
type ReplyInThreadRequest struct {
	Text             string   `json:"text,omitempty"`
	Html             string   `json:"html,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	Cc               []string `json:"cc,omitempty"`
	Bcc              []string `json:"bcc,omitempty"`
	ReplyAll         bool     `json:"reply_all,omitempty"`
	RequiresApproval bool     `json:"requires_approval,omitempty"`
}

// ForwardInThreadRequest is the payload for Threads.Emails.Forward. Text or
// Html is a note above the quoted original, whose attachments go along.
type ForwardInThreadRequest struct {
	To               []string `json:"to"`
	Cc               []string `json:"cc,omitempty"`
	Bcc              []string `json:"bcc,omitempty"`
	Text             string   `json:"text,omitempty"`
	Html             string   `json:"html,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	RequiresApproval bool     `json:"requires_approval,omitempty"`
}

// InboxDraft is a draft, as returned by Drafts.Create, Get, Update and
// Reject, and as a List row (rows carry Snippet instead of Html and Text).
// Status is "draft", "pending_approval", "sent", "rejected" or "expired";
// ApprovalReasons say why the inbox's rules hold it.
type InboxDraft struct {
	Object          string   `json:"object"`
	Id              string   `json:"id"`
	Type            string   `json:"type"` // "standalone" | "reply"
	To              []string `json:"to"`
	Cc              []string `json:"cc"`
	Bcc             []string `json:"bcc"`
	Subject         string   `json:"subject"`
	Html            string   `json:"html"`
	Text            string   `json:"text"`
	Snippet         string   `json:"snippet"`
	ThreadId        string   `json:"thread_id"`
	ReplyToEmailId  string   `json:"reply_to_email_id"`
	EmailId         string   `json:"email_id"` // set once sent
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	Status          string   `json:"status"`
	ApprovalReasons []string `json:"approval_reasons"`
	Forward         bool     `json:"forward"`
	DecidedBy       string   `json:"decided_by"`
	DecidedAt       string   `json:"decided_at"`
	ExpiresAt       string   `json:"expires_at"`
}

// CreateInboxDraftRequest is the payload for Drafts.Create. A reply draft
// sets ThreadId and ReplyToEmailId together; an unsent reply draft to the same
// email is returned instead of a new one.
type CreateInboxDraftRequest struct {
	To             []string `json:"to,omitempty"`
	Cc             []string `json:"cc,omitempty"`
	Bcc            []string `json:"bcc,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Html           string   `json:"html,omitempty"`
	Text           string   `json:"text,omitempty"`
	ThreadId       string   `json:"thread_id,omitempty"`
	ReplyToEmailId string   `json:"reply_to_email_id,omitempty"`
}

// UpdateInboxDraftRequest is the payload for Drafts.Update; empty fields are
// left unchanged, and setting Html or Text replaces both. The Clear… methods
// send null to empty a field; ClearBody empties Html and Text.
type UpdateInboxDraftRequest struct {
	To      []string `json:"to,omitempty"`
	Cc      []string `json:"cc,omitempty"`
	Bcc     []string `json:"bcc,omitempty"`
	Subject string   `json:"subject,omitempty"`
	Html    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`

	nulls []string
}

// ClearTo sends to as null, removing every To recipient.
func (r *UpdateInboxDraftRequest) ClearTo() { r.nulls = append(r.nulls, "to") }

// ClearCc sends cc as null.
func (r *UpdateInboxDraftRequest) ClearCc() { r.nulls = append(r.nulls, "cc") }

// ClearBcc sends bcc as null.
func (r *UpdateInboxDraftRequest) ClearBcc() { r.nulls = append(r.nulls, "bcc") }

// ClearSubject sends subject as null.
func (r *UpdateInboxDraftRequest) ClearSubject() { r.nulls = append(r.nulls, "subject") }

// ClearBody sends html and text as null, removing the draft's body.
func (r *UpdateInboxDraftRequest) ClearBody() { r.nulls = append(r.nulls, "html", "text") }

// MarshalJSON adds the cleared fields as explicit nulls.
func (r UpdateInboxDraftRequest) MarshalJSON() ([]byte, error) {
	type plain UpdateInboxDraftRequest
	return marshalWithNulls(plain(r), r.nulls)
}

// InboxDraftSent is returned by Drafts.Send once the draft went out.
type InboxDraftSent struct {
	Object   string `json:"object"`
	Id       string `json:"id"`
	ThreadId string `json:"thread_id"`
	EmailId  string `json:"email_id"`
}

// InboxesService covers /inboxes: agent and human inboxes (Resend's Inboxes
// beta plus MillionSend's Safety, Codes and Quarantine extensions).
type InboxesService struct {
	client *Client

	Safety     *InboxSafetyService
	Codes      *InboxCodesService
	Quarantine *InboxQuarantineService
	Labels     *InboxLabelsService
	Threads    *InboxThreadsService
	Drafts     *InboxDraftsService
}

// Create adds an inbox. Pass nil for a random address on the shared domain.
//
// On the shared domain a chosen Username or FromName is reviewed first. While
// that review is unavailable the answer is 503 name_review_unavailable with
// RetryAfter 60 (leaving Username out still gives a random address); while a
// new team's subdomain waits for its own review, every shared or forwarding
// inbox gets that 503 with RetryAfter 300. Past the daily review budget it is
// 429 rate_limit_exceeded until the next UTC midnight.
//
// Create takes no Idempotency-Key. After a 503 or 429 nothing was created,
// so sending it again is safe; after a timeout a retry may create a second
// inbox, and with a Username a 409 on the retry usually means the first one
// worked (Get finds it by address).
func (s *InboxesService) Create(params *CreateInboxRequest) (*Inbox, error) {
	if params == nil {
		params = &CreateInboxRequest{}
	}
	return doJSON[Inbox](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: "/inboxes", body: params,
	})
}

// List returns the team's inboxes, newest first. Pass nil for defaults.
func (s *InboxesService) List(opts *ListInboxesOptions) (*ListResponse[Inbox], error) {
	var query url.Values
	if opts != nil {
		query = (&ListOptions{Limit: opts.Limit, After: opts.After, Before: opts.Before}).values()
		setQuery(query, "kind", opts.Kind)
	}
	return doJSON[ListResponse[Inbox]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/inboxes", query: query,
	})
}

// Get fetches an inbox by id or email address.
func (s *InboxesService) Get(idOrEmail string) (*Inbox, error) {
	return doJSON[Inbox](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: inboxPath(idOrEmail),
	})
}

// Update renames an inbox or changes its display name or expiry. On the shared
// domain a new FromName is reviewed as on Create (503 or 429).
func (s *InboxesService) Update(id string, params *UpdateInboxRequest) (*Inbox, error) {
	return doJSON[Inbox](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: inboxPath(id), body: params,
	})
}

// Remove deletes an inbox with its threads, drafts and labels, and revokes
// its inbox-scoped keys.
func (s *InboxesService) Remove(id string) (*RemoveResponse, error) {
	return doJSON[RemoveResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: inboxPath(id),
	})
}

// InboxSafetyService covers /inboxes/:id/safety.
type InboxSafetyService struct{ client *Client }

// Get fetches an inbox's rules.
func (s *InboxSafetyService) Get(inboxId string) (*InboxSafety, error) {
	return doJSON[InboxSafety](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: inboxPath(inboxId) + "/safety",
	})
}

// Update changes some of an inbox's rules and returns all of them.
// Inbox-scoped keys get 403.
func (s *InboxSafetyService) Update(inboxId string, params *InboxSafetyUpdate) (*InboxSafety, error) {
	return doJSON[InboxSafety](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: inboxPath(inboxId) + "/safety", body: params,
	})
}

// InboxCodesService covers /inboxes/:id/codes.
type InboxCodesService struct{ client *Client }

// Latest returns the newest verification code or sign-in link the inbox
// received. Pass nil for defaults.
func (s *InboxCodesService) Latest(inboxId string, opts *LatestCodeOptions) (*VerificationCode, error) {
	return s.LatestWithContext(context.Background(), inboxId, opts)
}

// LatestWithContext is Latest with a caller-supplied context, which can
// cancel its wait.
func (s *InboxCodesService) LatestWithContext(ctx context.Context, inboxId string, opts *LatestCodeOptions) (*VerificationCode, error) {
	var query url.Values
	var wait time.Duration
	if opts != nil {
		query = url.Values{}
		setQuery(query, "from", opts.From)
		setQuery(query, "wait", opts.Wait)
		setQuery(query, "max_age", opts.MaxAge)
		setQuery(query, "since", opts.Since)
		wait = time.Duration(opts.Wait) * time.Second
	}
	return doJSON[VerificationCode](s.client, ctx, requestParams{
		method: http.MethodGet, path: inboxPath(inboxId) + "/codes/latest", query: query, wait: wait,
	})
}

// InboxQuarantineService covers /inboxes/:id/quarantine.
type InboxQuarantineService struct{ client *Client }

// List returns the inbox's held messages, newest first. Pass nil for defaults.
func (s *InboxQuarantineService) List(inboxId string, opts *ListOptions) (*ListResponse[QuarantinedEmail], error) {
	return doJSON[ListResponse[QuarantinedEmail]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: inboxPath(inboxId) + "/quarantine", query: opts.values(),
	})
}

// Release delivers a held message to the inbox. Pass nil to release without
// trusting the sender.
func (s *InboxQuarantineService) Release(inboxId, emailId string, params *ReleaseQuarantinedRequest) (*QuarantineRelease, error) {
	if params == nil {
		params = &ReleaseQuarantinedRequest{}
	}
	return doJSON[QuarantineRelease](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: inboxPath(inboxId) + "/quarantine/" + url.PathEscape(emailId) + "/release",
		body: params,
	})
}

// InboxLabelsService covers /inboxes/:id/labels.
type InboxLabelsService struct{ client *Client }

// List returns every label of the inbox, oldest first (unpaginated).
func (s *InboxLabelsService) List(inboxId string) (*ListResponse[InboxLabel], error) {
	return doJSON[ListResponse[InboxLabel]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: inboxPath(inboxId) + "/labels",
	})
}

// Create adds a label to the inbox.
func (s *InboxLabelsService) Create(inboxId string, params *CreateInboxLabelRequest) (*InboxLabel, error) {
	return doJSON[InboxLabel](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: inboxPath(inboxId) + "/labels", body: params,
	})
}

// Update renames or recolors a label.
func (s *InboxLabelsService) Update(inboxId, labelId string, params *UpdateInboxLabelRequest) (*InboxLabel, error) {
	return doJSON[InboxLabel](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: inboxPath(inboxId) + "/labels/" + url.PathEscape(labelId), body: params,
	})
}

// Remove deletes a label and takes it off every thread.
func (s *InboxLabelsService) Remove(inboxId, labelId string) (*RemoveResponse, error) {
	return doJSON[RemoveResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: inboxPath(inboxId) + "/labels/" + url.PathEscape(labelId),
	})
}

// InboxThreadsService covers /inboxes/:id/threads. Emails covers the
// messages of a thread.
type InboxThreadsService struct {
	client *Client

	Emails *InboxThreadEmailsService
}

// List returns the inbox's threads, most recently active first. Pass nil for
// defaults.
func (s *InboxThreadsService) List(inboxId string, opts *ListInboxThreadsOptions) (*ListResponse[InboxThreadListItem], error) {
	return doJSON[ListResponse[InboxThreadListItem]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: inboxPath(inboxId) + "/threads", query: opts.values(),
	})
}

// Search finds threads by subject, sender and recipients.
func (s *InboxThreadsService) Search(inboxId string, opts *SearchInboxThreadsOptions) (*ListResponse[InboxThreadListItem], error) {
	return doJSON[ListResponse[InboxThreadListItem]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: inboxPath(inboxId) + "/threads/search", query: opts.values(),
	})
}

// Get fetches a thread (without its messages; see Emails.List).
func (s *InboxThreadsService) Get(inboxId, threadId string) (*InboxThread, error) {
	return doJSON[InboxThread](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: threadPath(inboxId, threadId),
	})
}

// Update marks a thread read or unread, moves it to a folder or labels it.
func (s *InboxThreadsService) Update(inboxId, threadId string, params *UpdateInboxThreadRequest) (*InboxThread, error) {
	return doJSON[InboxThread](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: threadPath(inboxId, threadId), body: params,
	})
}

// Remove moves a thread to trash, purged after 30 days. Deleted is false when
// it was in trash already.
func (s *InboxThreadsService) Remove(inboxId, threadId string) (*RemoveResponse, error) {
	return doJSON[RemoveResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: threadPath(inboxId, threadId),
	})
}

// InboxThreadEmailsService covers /inboxes/:id/threads/:thread_id/emails.
type InboxThreadEmailsService struct{ client *Client }

// List returns a thread's messages, oldest first; After pages toward newer.
// Pass nil for defaults; WithIncludeQuarantined also lists held messages.
func (s *InboxThreadEmailsService) List(inboxId, threadId string, opts *ListOptions, ropts ...RequestOption) (*ListResponse[InboxThreadEmail], error) {
	return doJSON[ListResponse[InboxThreadEmail]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: threadPath(inboxId, threadId) + "/emails", query: heldQuery(opts.values(), ropts),
	})
}

// Get fetches one message of a thread; WithIncludeQuarantined reads it even
// when it is held.
func (s *InboxThreadEmailsService) Get(inboxId, threadId, emailId string, opts ...RequestOption) (*InboxThreadEmail, error) {
	return doJSON[InboxThreadEmail](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: threadPath(inboxId, threadId) + "/emails/" + url.PathEscape(emailId),
		query: heldQuery(nil, opts),
	})
}

// heldQuery adds include_quarantined=true to q when opts carry WithIncludeQuarantined.
func heldQuery(q url.Values, opts []RequestOption) url.Values {
	if !buildConfig(opts).includeQuarantined {
		return q
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("include_quarantined", "true")
	return q
}

// Reply answers a message in its thread from the inbox address. Pass
// WithIdempotencyKey to make it idempotent. The outcome's Draft is set
// instead of Sent when the inbox's rules hold the reply for approval. From a
// shared inbox the shared row's SendingMode applies (see Domain): a send it
// forbids is 403 inbox_send_policy, the team's daily cap 429
// inbox_send_cap_reached, and more than 10 recipients 422.
func (s *InboxThreadEmailsService) Reply(inboxId, threadId, emailId string, params *ReplyInThreadRequest, opts ...RequestOption) (*SendOutcome[InboxThreadEmail], error) {
	return doOutcome[InboxThreadEmail](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: threadPath(inboxId, threadId) + "/emails/" + url.PathEscape(emailId) + "/reply",
		body: params, idempotencyKey: buildConfig(opts).idempotencyKey,
	})
}

// Forward sends a message of a thread on. Same options and outcome as Reply.
func (s *InboxThreadEmailsService) Forward(inboxId, threadId, emailId string, params *ForwardInThreadRequest, opts ...RequestOption) (*SendOutcome[InboxThreadEmail], error) {
	return doOutcome[InboxThreadEmail](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: threadPath(inboxId, threadId) + "/emails/" + url.PathEscape(emailId) + "/forward",
		body: params, idempotencyKey: buildConfig(opts).idempotencyKey,
	})
}

// InboxDraftsService covers /inboxes/:id/drafts.
type InboxDraftsService struct{ client *Client }

// List returns the inbox's unsent drafts, most recently updated first. Pass
// nil for defaults.
func (s *InboxDraftsService) List(inboxId string, opts *ListOptions) (*ListResponse[InboxDraft], error) {
	return doJSON[ListResponse[InboxDraft]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: inboxPath(inboxId) + "/drafts", query: opts.values(),
	})
}

// Create saves a draft; nothing is sent until Send. On a shared inbox a draft
// takes at most 10 distinct recipients (422).
func (s *InboxDraftsService) Create(inboxId string, params *CreateInboxDraftRequest) (*InboxDraft, error) {
	return doJSON[InboxDraft](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: inboxPath(inboxId) + "/drafts", body: params,
	})
}

// Get fetches a draft, also once sent.
func (s *InboxDraftsService) Get(inboxId, draftId string) (*InboxDraft, error) {
	return doJSON[InboxDraft](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: draftPath(inboxId, draftId),
	})
}

// Update edits an unsent draft.
func (s *InboxDraftsService) Update(inboxId, draftId string, params *UpdateInboxDraftRequest) (*InboxDraft, error) {
	return doJSON[InboxDraft](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: draftPath(inboxId, draftId), body: params,
	})
}

// Remove deletes an unsent draft.
func (s *InboxDraftsService) Remove(inboxId, draftId string) (*RemoveResponse, error) {
	return doJSON[RemoveResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: draftPath(inboxId, draftId),
	})
}

// Send sends a draft. Pass WithIdempotencyKey to make it idempotent. The
// outcome's Draft is set instead of Sent when the inbox's rules hold it for
// approval; an inbox-scoped key gets 403 approval_required for a draft that
// waits for a person. A person or a team key skips the inbox's rules, never
// the shared domain's (see InboxThreadEmailsService.Reply).
func (s *InboxDraftsService) Send(inboxId, draftId string, opts ...RequestOption) (*SendOutcome[InboxDraftSent], error) {
	return doOutcome[InboxDraftSent](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: draftPath(inboxId, draftId) + "/send",
		idempotencyKey: buildConfig(opts).idempotencyKey,
	})
}

// Reject turns down a draft waiting for approval (approvers only).
func (s *InboxDraftsService) Reject(inboxId, draftId string) (*InboxDraft, error) {
	return doJSON[InboxDraft](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: draftPath(inboxId, draftId) + "/reject",
	})
}

func inboxPath(id string) string {
	return "/inboxes/" + url.PathEscape(id)
}

func threadPath(inboxId, threadId string) string {
	return inboxPath(inboxId) + "/threads/" + url.PathEscape(threadId)
}

func draftPath(inboxId, draftId string) string {
	return inboxPath(inboxId) + "/drafts/" + url.PathEscape(draftId)
}
