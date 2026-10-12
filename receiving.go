package millionsend

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ReceivedEmail is a message one of the team's domains received: a row of
// Receiving.List, or the full record of Receiving.Get, which adds Object,
// Html, HtmlFormat, Text, Headers, Authentication, Raw, ExtractedText,
// SpamScore and VirusSignature. Verdicts and tiers are plain strings, so
// values this SDK version has never seen decode fine.
type ReceivedEmail struct {
	Object      string            `json:"object"`
	Id          string            `json:"id"`
	From        string            `json:"from"` // bare address; the display name is in Headers["from"]
	To          []string          `json:"to"`
	Cc          []string          `json:"cc"`
	Bcc         []string          `json:"bcc"`
	ReplyTo     []string          `json:"reply_to"`
	ReceivedFor []string          `json:"received_for"` // the team's envelope recipients
	Subject     string            `json:"subject"`
	MessageId   string            `json:"message_id"`
	CreatedAt   string            `json:"created_at"`
	Attachments []EmailAttachment `json:"attachments"`

	Html           string                       `json:"html"`
	HtmlFormat     string                       `json:"html_format"`
	Text           string                       `json:"text"` // invisible characters removed
	Headers        map[string]string            `json:"headers"`
	Authentication *ReceivedEmailAuthentication `json:"authentication"`
	// Raw links the original RFC 822 message; nil once retention removed the
	// content, and for a message carrying malware.
	Raw            *ReceivedEmailRaw `json:"raw"`
	ExtractedText  string            `json:"extracted_text"` // the visible reply, hidden text removed
	SpamScore      *int              `json:"spam_score"`
	VirusSignature string            `json:"virus_signature"`

	// InboxId and ThreadId place the message in an inbox: the calling key's,
	// else its first. Empty for catch-all mail.
	InboxId      string                  `json:"inbox_id"`
	ThreadId     string                  `json:"thread_id"`
	Inboxes      []ReceivedEmailInbox    `json:"inboxes"` // full-access views only
	Screening    *Screening              `json:"screening"`
	SpamVerdict  string                  `json:"spam_verdict"`  // "pass" | "gray" | "fail"; "" when not computed
	VirusVerdict string                  `json:"virus_verdict"` // "pass" | "fail" | "processing_failed"; "" when not scanned
	Extracted    *ReceivedEmailExtracted `json:"extracted"`
	Read         *bool                   `json:"read"` // set with an inbox context
}

// EmailAttachment is a file of a received or inbox message. Message reads
// (ReceivedEmail.Attachments, InboxThreadEmail.Attachments) leave DownloadUrl
// empty; Receiving.Attachments.List and Get add a signed DownloadUrl, valid
// for an hour without an API key, and its ExpiresAt. On mail an inbox sent,
// Id is the attachment's position ("0", "1"…).
type EmailAttachment struct {
	Object             string `json:"object"`
	Id                 string `json:"id"`
	Filename           string `json:"filename"`
	ContentType        string `json:"content_type"`
	ContentDisposition string `json:"content_disposition"`
	ContentId          string `json:"content_id"`
	Size               int64  `json:"size"`
	DownloadUrl        string `json:"download_url"`
	ExpiresAt          string `json:"expires_at"`
}

// ReceivedEmailAuthentication holds the SPF, DKIM and DMARC results checked on
// arrival: "pass", "fail", "gray", "processing_failed" or "unknown".
type ReceivedEmailAuthentication struct {
	Spf   string `json:"spf"`
	Dkim  string `json:"dkim"`
	Dmarc string `json:"dmarc"`
}

// ReceivedEmailRaw is a signed link to the original message, valid for an hour.
type ReceivedEmailRaw struct {
	DownloadUrl string `json:"download_url"`
	ExpiresAt   string `json:"expires_at"`
}

// Screening is how an inbox screened a message. Lists carry Verdict and
// SenderTrust; message reads carry every field.
type Screening struct {
	Verdict           string   `json:"verdict"`           // "pending" | "clean" | "suspicious" | "quarantined" | "released"
	SenderTrust       string   `json:"sender_trust"`      // "owner" | "allowlisted" | "known_thread" | "external" | "unauthenticated" | "failed"
	QuarantineReason  string   `json:"quarantine_reason"` // set when quarantined or released
	Reasons           []string `json:"reasons"`
	InjectionScore    *int     `json:"injection_score"` // 0-100; nil when not judged
	PhishingScore     *int     `json:"phishing_score"`
	HiddenTextRemoved bool     `json:"hidden_text_removed"`
}

// ReceivedEmailInbox is one inbox a received email went to.
type ReceivedEmailInbox struct {
	InboxId  string `json:"inbox_id"`
	ThreadId string `json:"thread_id"`
	Verdict  string `json:"verdict"`
}

// ReceivedEmailExtracted is what verification mail yields. Lists carry Kinds
// ("code", "link"); Receiving.Get adds the Code or the sign-in Link, which
// are never served from held mail.
type ReceivedEmailExtracted struct {
	Kinds                   []string `json:"kinds"`
	Code                    string   `json:"code"`
	Link                    string   `json:"link"`
	LinkDomainMatchesSender *bool    `json:"link_domain_matches_sender"`
	Ambiguous               bool     `json:"ambiguous"`
}

// ReceivedEmailList is a page of Receiving.List. On a long-poll, Held lists
// the matches held for screening or in quarantine since the cursor, ids only.
type ReceivedEmailList struct {
	Object  string          `json:"object"`
	Data    []ReceivedEmail `json:"data"`
	HasMore bool            `json:"has_more"`
	Held    []HeldEmail     `json:"held"`
}

// HeldEmail is a long-poll match an inbox held back. Reason is "pending"
// (still being screened) or "quarantined"; QuarantineReason says why, empty
// while pending. InboxId is empty for a catch-all copy held for a virus.
type HeldEmail struct {
	Id               string `json:"id"`
	InboxId          string `json:"inbox_id"`
	Reason           string `json:"reason"`
	QuarantineReason string `json:"quarantine_reason"`
	CreatedAt        string `json:"created_at"`
}

// ListReceivedEmailsOptions filters and paginates Receiving.List. Wait (up to
// 50 seconds) long-polls for mail newer than Before or Since (default: a
// minute before the call) and answers an empty page when nothing arrives.
type ListReceivedEmailsOptions struct {
	Limit           int
	After           string
	Before          string
	Wait            int
	Since           string // ISO 8601
	InboxId         string
	To              string // an address the message was delivered to
	From            string // an address, or a domain covering its subdomains
	SubjectContains string
	Unread          *bool // needs InboxId
	// IncludeQuarantined also lists held mail; full-access keys only.
	IncludeQuarantined bool
}

func (o *ListReceivedEmailsOptions) values() url.Values {
	if o == nil {
		return nil
	}
	v := (&ListOptions{Limit: o.Limit, After: o.After, Before: o.Before}).values()
	setQuery(v, "wait", o.Wait)
	setQuery(v, "since", o.Since)
	setQuery(v, "inbox_id", o.InboxId)
	setQuery(v, "to", o.To)
	setQuery(v, "from", o.From)
	setQuery(v, "subject_contains", o.SubjectContains)
	setQuery(v, "unread", o.Unread)
	setQuery(v, "include_quarantined", o.IncludeQuarantined)
	return v
}

// GetReceivedEmailParams holds the options of Receiving.GetWithOptions, as in
// resend-go. HtmlFormat is "data_uri" (default: images referenced by cid:
// inlined) or "cid" (references kept; the parts are in Attachments).
type GetReceivedEmailParams struct {
	HtmlFormat *string
}

// ReplyReceivedEmailRequest is the payload for Receiving.Reply: Text and/or
// Html, plus optional Subject (default "Re: " and the original's), Cc, Bcc,
// ReplyAll, RequiresApproval, From (a verified team address; default the
// inbox's) and InboxId (which of the message's inboxes replies).
type ReplyReceivedEmailRequest struct {
	Text             string   `json:"text,omitempty"`
	Html             string   `json:"html,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	Cc               []string `json:"cc,omitempty"`
	Bcc              []string `json:"bcc,omitempty"`
	ReplyAll         bool     `json:"reply_all,omitempty"`
	RequiresApproval bool     `json:"requires_approval,omitempty"`
	From             string   `json:"from,omitempty"`
	InboxId          string   `json:"inbox_id,omitempty"`
}

// ForwardReceivedEmailRequest is the payload for Receiving.Forward. Text or
// Html is a note above the quoted original. Mode "attachment" sends the
// original as forwarded_message.eml instead of quoting it inline.
type ForwardReceivedEmailRequest struct {
	To               []string `json:"to"`
	Cc               []string `json:"cc,omitempty"`
	Bcc              []string `json:"bcc,omitempty"`
	Text             string   `json:"text,omitempty"`
	Html             string   `json:"html,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	RequiresApproval bool     `json:"requires_approval,omitempty"`
	From             string   `json:"from,omitempty"`
	InboxId          string   `json:"inbox_id,omitempty"`
	Mode             string   `json:"mode,omitempty"` // "inline" (default) | "attachment"
}

// ReceivedEmailSent is the email a Receiving.Reply or Forward sent.
type ReceivedEmailSent struct {
	Object   string `json:"object"`
	Id       string `json:"id"`
	ThreadId string `json:"thread_id"`
	InboxId  string `json:"inbox_id"`
}

// RemoveResponse is returned by the receiving and inbox removals. Deleted is
// false only for a thread that was in trash already.
type RemoveResponse struct {
	Object  string `json:"object"`
	Id      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// ReceivingService covers /emails/receiving: received mail, its long-poll,
// server-side reply and forward, and erase. Inbox-scoped keys never see mail
// held for screening or in quarantine.
type ReceivingService struct {
	client *Client

	Attachments *ReceivingAttachmentsService
}

// List returns received mail, newest first. Pass nil for defaults.
func (s *ReceivingService) List(opts *ListReceivedEmailsOptions) (*ReceivedEmailList, error) {
	return s.ListWithContext(context.Background(), opts)
}

// ListWithContext is List with a caller-supplied context, which can cancel a
// long-poll.
func (s *ReceivingService) ListWithContext(ctx context.Context, opts *ListReceivedEmailsOptions) (*ReceivedEmailList, error) {
	var wait time.Duration
	if opts != nil {
		wait = time.Duration(opts.Wait) * time.Second
	}
	return doJSON[ReceivedEmailList](s.client, ctx, requestParams{
		method: http.MethodGet, path: "/emails/receiving", query: opts.values(), wait: wait,
	})
}

// Get fetches a received email with its content.
func (s *ReceivingService) Get(id string) (*ReceivedEmail, error) {
	return s.GetWithOptions(context.Background(), id, nil)
}

// GetWithOptions is Get with a context and the HtmlFormat option.
func (s *ReceivingService) GetWithOptions(ctx context.Context, id string, params *GetReceivedEmailParams) (*ReceivedEmail, error) {
	var query url.Values
	if params != nil && params.HtmlFormat != nil {
		query = url.Values{"html_format": {*params.HtmlFormat}}
	}
	return doJSON[ReceivedEmail](s.client, ctx, requestParams{
		method: http.MethodGet, path: receivedPath(id), query: query,
	})
}

// Reply answers a received email in its thread. Pass WithIdempotencyKey to
// make it idempotent. The outcome's Draft is set instead of Sent when an
// inbox's rules hold the reply for a person to approve.
func (s *ReceivingService) Reply(id string, params *ReplyReceivedEmailRequest, opts ...RequestOption) (*SendOutcome[ReceivedEmailSent], error) {
	return doOutcome[ReceivedEmailSent](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: receivedPath(id) + "/reply", body: params,
		idempotencyKey: buildConfig(opts).idempotencyKey,
	})
}

// Forward sends a received email on. Same options and outcome as Reply.
func (s *ReceivingService) Forward(id string, params *ForwardReceivedEmailRequest, opts ...RequestOption) (*SendOutcome[ReceivedEmailSent], error) {
	return doOutcome[ReceivedEmailSent](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: receivedPath(id) + "/forward", body: params,
		idempotencyKey: buildConfig(opts).idempotencyKey,
	})
}

// Remove erases a received email now (full-access keys not scoped to an inbox).
func (s *ReceivingService) Remove(id string) (*RemoveResponse, error) {
	return doJSON[RemoveResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: receivedPath(id),
	})
}

// ReceivingAttachmentsService covers /emails/receiving/:id/attachments.
type ReceivingAttachmentsService struct{ client *Client }

// List returns a received email's attachments with their download links.
// Pass nil for defaults.
func (s *ReceivingAttachmentsService) List(emailId string, opts *ListOptions) (*ListResponse[EmailAttachment], error) {
	return doJSON[ListResponse[EmailAttachment]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: receivedPath(emailId) + "/attachments", query: opts.values(),
	})
}

// Get fetches one attachment of a received email with its download link.
func (s *ReceivingAttachmentsService) Get(emailId, id string) (*EmailAttachment, error) {
	return doJSON[EmailAttachment](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: receivedPath(emailId) + "/attachments/" + url.PathEscape(id),
	})
}

func receivedPath(id string) string {
	return "/emails/receiving/" + url.PathEscape(id)
}
