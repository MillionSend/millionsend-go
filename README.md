# millionsend-go

Official Go client for [MillionSend](https://github.com/MillionSend/millionsend) — a
self-hostable, Resend-compatible email API on AWS SES.

The API is wire-compatible with Resend, and this SDK mirrors the shape of
[`resend-go`](https://github.com/resend/resend-go), so migrating is mostly a
find-and-replace: swap the import and constructor. MillionSend Cloud works with
just the API key; a self-hosted instance also sets `BaseURL`.

## Install

```bash
go get github.com/MillionSend/millionsend-go
```

Requires Go 1.21+.

## Quickstart

```go
package main

import (
	"fmt"
	"log"

	millionsend "github.com/MillionSend/millionsend-go"
)

func main() {
	client := millionsend.NewClient("ms_123")
	// client.BaseURL = "https://mail.acme.dev" // self-hosted instances only

	sent, err := client.Emails.Send(&millionsend.SendEmailRequest{
		From:    "Acme <onboarding@acme.dev>",
		To:      []string{"success@simulator.amazonses.com"},
		Subject: "Hello from MillionSend",
		Html:    "<strong>It works!</strong>",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("sent", sent.Id)
}
```

## Configuration

```go
client := millionsend.NewClient(apiKey)
client.BaseURL = "https://mail.acme.dev"        // self-hosted instance
client.HTTPClient = &http.Client{Timeout: time.Minute} // bring your own transport
client.UserAgent = "myapp/1.0 " + client.UserAgent      // extend the User-Agent
client.AllowInsecureHTTP = true                          // accept a non-loopback http:// BaseURL
```

- `apiKey` falls back to `MILLIONSEND_API_KEY` when empty.
- `BaseURL` falls back to `MILLIONSEND_BASE_URL`, then `https://api.millionsend.com`
  (MillionSend Cloud). A self-hosted instance sets its own origin.
- Plain `http://` is only accepted for loopback hosts (`localhost`, `127.0.0.1`, `::1`);
  any other `http://` URL makes every call return an `application_error`, since the API
  key is sent as a bearer header. Set `AllowInsecureHTTP = true` to talk to a non-TLS
  instance elsewhere (e.g. inside a private network).

Every method has a non-context form. The transactional hot path — every
method of `Emails` itself, `Batch.Send`, `Contacts.Batch.*`,
`Suppressions.Batch.*`, `Deliverability.Get` and `Usage.Get` — also exposes a
`…WithContext(ctx, …)` variant, as do the long-polls
`Emails.Receiving.ListWithContext` and `Inboxes.Codes.LatestWithContext`
(`Emails.Receiving.GetWithOptions` takes a context too). A long-poll stretches
the `HTTPClient` timeout by its `Wait`, so the default 30s client can wait the
full 50s.

## Errors

Methods return `(*T, error)`. On any non-2xx response the error is a
`*MillionSendError`:

```go
sent, err := client.Emails.Get(id)
if err != nil {
	var apiErr *millionsend.MillionSendError
	if errors.As(err, &apiErr) {
		switch apiErr.Name { // stable, switchable code
		case "not_found":
			// …
		case "validation_error":
			// …
		case "all_recipients_suppressed":
			// nothing was sent: see below
		}
		log.Printf("status=%d name=%s: %s", apiErr.StatusCode, apiErr.Name, apiErr.Message)
	}
}
```

`StatusCode` is `0` when the request never reached the API (a transport or
client-side failure) — the wire's `statusCode: null`. `RetryAfter` is the
response's `retry-after` header in seconds (`*int`, nil when it has none).
`Message` is for people: switch on `Name`, don't parse the message.

Receiving and inboxes add the names `inbox_send_policy`, `inbox_send_cap_reached`,
`approval_required`, `email_quarantined`, `email_blocked`, `too_many_waits`,
`plan_limit_reached` and `name_review_unavailable` (503, see
[the shared domain](#the-shared-domain)). Changes to the shared domain's row
in `Domains` answer `403 forbidden`.

An unknown route answers `404 not_found`. Servers before v0.13.0 answer it in
plain text, which surfaces as `application_error` with `StatusCode` 404.

`Emails.Send` and `Batch.Send` answer `422 all_recipients_suppressed` when every
`To` recipient is on the suppression list or opted out of the send's `TopicId`;
nothing is sent.

### Retries

The SDK never retries on its own. Worth retrying after `RetryAfter` seconds:
`503 name_review_unavailable`, `429 too_many_waits`, and `429
rate_limit_exceeded` when `RetryAfter` is at most 60 (the per-minute limits).
Not worth retrying in the same process: `429 inbox_send_cap_reached` and the
daily name-review `429` (also `rate_limit_exceeded`, so check `RetryAfter`,
not just the name), which both last until the next UTC midnight, and any 403,
404, 409 or 422.

```go
var apiErr *millionsend.MillionSendError
if errors.As(err, &apiErr) && apiErr.RetryAfter != nil &&
	(apiErr.Name == "name_review_unavailable" || apiErr.Name == "too_many_waits" ||
		apiErr.Name == "rate_limit_exceeded" && *apiErr.RetryAfter <= 60) {
	time.Sleep(time.Duration(*apiErr.RetryAfter) * time.Second) // then try again
}
```

Retry sends, replies, forwards and `Inboxes.Drafts.Send` with the same
`WithIdempotencyKey`. A `Draft` outcome (HTTP 202) is a message held for
approval, not a failure: never send it again.

## Request options

`Emails.Send`, `Batch.Send`, `Contacts.Batch.Create`, `Contacts.Batch.Get`,
`Contacts.List`, `Segments.ListContacts` and `Contacts.Remove` take functional
options, and so do the replies, forwards and draft sends of
`Emails.Receiving` and `Inboxes` (`WithIdempotencyKey`) and
`Inboxes.Threads.Emails.List/Get` (`WithIncludeQuarantined`):

```go
client.Emails.Send(req, millionsend.WithIdempotencyKey("order-42"))
client.Batch.Send(reqs,
	millionsend.WithIdempotencyKey("run-7"),
	millionsend.WithBatchValidation(millionsend.BatchValidationPermissive), // x-batch-validation header
)
client.Contacts.Batch.Create(contacts, millionsend.WithOnConflict(millionsend.OnConflictUpsert))
client.Contacts.List(nil, millionsend.WithInclude(millionsend.ContactIncludeTopics)) // ?include=topics
client.Contacts.Remove(addr, millionsend.WithErase())                                // ?erase=true
client.Inboxes.Threads.Emails.List(inboxID, threadID, nil, millionsend.WithIncludeQuarantined()) // held mail too
```

resend-go's option structs work too:

```go
client.Emails.SendWithOptions(ctx, req, &millionsend.SendEmailOptions{IdempotencyKey: "order-42"})
client.Batch.SendWithOptions(ctx, reqs, &millionsend.BatchSendEmailOptions{
	IdempotencyKey:  "run-7",
	BatchValidation: millionsend.BatchValidationPermissive,
})
```

Batch validation is `strict` by default (any invalid item rejects the whole
batch). `permissive` sends the valid items and lists the rest in the
response's `Errors []BatchError{Index, StatusCode, Name, Message}`;
`StatusCode` and `Name`, what strict mode would answer for the item, come
from `Contacts.Batch.Create` only.

## Clearing a value with `null`

Update requests use `omitempty`, so an empty field is simply left unchanged.
Where the API accepts an explicit `null` to erase a stored value, the request
has a `Clear…` method that puts that `null` on the wire:

```go
req := &millionsend.UpdateContactRequest{Id: contactID, FirstName: millionsend.Ptr("Ada")}
req.ClearLastName() // {"first_name":"Ada","last_name":null}
client.Contacts.Update(req)
```

Available: `UpdateContactRequest.ClearFirstName/ClearLastName`,
`UpdateBroadcastRequest.ClearTopicId`, `UpdateTemplateRequest.ClearSubject/ClearText/ClearAlias`,
`UpdateDomainRequest.ClearTrackingSubdomain`, `UpdateSegmentRequest.ClearFilter`,
`UpdateWebhookRequest.ClearInboxIds`, `UpdateInboxRequest.ClearExpiresIn`,
`InboxSafetyUpdate.ClearSendCap`, `UpdateInboxDraftRequest.ClearTo/ClearCc/ClearBcc/ClearSubject/ClearBody`. A `nil` value inside
`UpdateContactRequest.Properties` removes that key, and a nil
`UpdateContactPropertyRequest.FallbackValue` clears the fallback.

## Pagination

Lists take `Limit` and at most one of `After` or `Before` (an item id), and
return `{Object, Data, HasMore}`. They are newest first, except threads (most
recently active first), a thread's emails (oldest first), drafts (most
recently updated first), a received email's attachments (in message order),
and labels and topics, which are not paginated. `After` takes the last id of
a page and moves toward older items; `Before` takes the first id and moves
toward newer ones, still newest first within the page. `HasMore` refers to
the direction you're moving in.

To fetch what arrived since your last sync, start `Before` at the newest id
you have:

```go
before := newestStoredID
for {
	page, err := client.Emails.List(&millionsend.ListOptions{Before: before, Limit: 100})
	if err != nil {
		return err
	}
	// store page.Data
	if !page.HasMore || len(page.Data) == 0 {
		break
	}
	before = page.Data[0].Id
}
```

A cursor whose item was deleted answers `422 validation_error` on the
top-level lists and `Emails.Receiving.List`, and `404 not_found` on the inbox
lists: start again without a cursor and stop at the ids you already have.
Servers before v0.13.0 list oldest first; walking every page with `After`
returns every item on both.

## Resources

### Emails

```go
client.Emails.Send(&millionsend.SendEmailRequest{
	From: "Acme <a@acme.dev>", To: []string{"b@x.dev"}, Subject: "Hi",
	Html: "<p>Hi</p>", Text: "Hi",
	Cc: []string{"cc@x.dev"}, Bcc: []string{"bcc@x.dev"}, ReplyTo: "r@acme.dev",
	ScheduledAt: "in 2 hours",
	Tags:        []millionsend.Tag{{Name: "campaign", Value: "launch"}},
	TopicId:     topicID, // recipients opted out of the topic are skipped
	Headers:     map[string]string{"X-Entity-Ref-ID": "123"},
	Attachments: []*millionsend.Attachment{{Filename: "hi.txt", Content: []byte("hello"), ContentType: "text/plain"}},
}, millionsend.WithIdempotencyKey("key"))
client.Emails.Get(id)         // includes Score (*float64; nil when no insights)
client.Emails.List(&millionsend.ListOptions{Limit: 50})
client.Emails.Update(&millionsend.UpdateEmailRequest{Id: id, ScheduledAt: "2026-09-10T09:00:00Z"}) // reschedule
client.Emails.Cancel(id)      // scheduled, unsent emails only
client.Emails.Remove(id)      // deletes the record and its events
client.Emails.GetInsights(id) // per-email deliverability report; 404 until computed

client.Batch.Send([]*millionsend.SendEmailRequest{a, b}, millionsend.WithIdempotencyKey("key")) // up to 100
```

`Attachment.Content` is raw bytes; it is base64-encoded on the wire. `Template`
is passed through unchanged: the API does not support stored-template sends
yet and answers `422` (send `Html`/`Text` instead).

`Subject` takes up to 998 characters. A `To`, `Cc` or `Bcc` address whose
domain can never receive mail (`example.com`, a reserved TLD such as `.test`
or `.invalid`, or a TLD that doesn't exist) gets `422 validation_error`; to
try sending without emailing anyone, send to `success@simulator.amazonses.com`.

`From` can be an inbox's address on [the shared domain](#the-shared-domain)
while its `SendingMode` is `open`: exactly the inbox's `EmailAddress`, with
no +tag, no display name or the inbox's `FromName`, at most 10 distinct
recipients across `To`, `Cc` and `Bcc`, and no `ReplyTo`, `Attachments`,
`Headers` or `ScheduledAt`. A key restricted to one of your domains gets
`403 restricted_api_key` for such a send.

`Batch.Send` items never send from the shared domain: such an item gets `403
inbox_send_policy`, which in strict mode fails the whole batch. A retry with
the same `WithIdempotencyKey` gets the stored response, even if a recipient
was refused or suppressed since.

### Receiving

Mail your domains receive: resend-go's `Emails.Receiving` plus MillionSend's
long-poll, server-side reply and forward, and erase. Inbox-scoped keys never
see mail held for screening or in quarantine.

```go
page, _ := client.Emails.Receiving.List(&millionsend.ListReceivedEmailsOptions{Limit: 20})
email, _ := client.Emails.Receiving.Get(id)
email, _ = client.Emails.Receiving.GetWithOptions(ctx, id, &millionsend.GetReceivedEmailParams{HtmlFormat: millionsend.Ptr("cid")})
// email.ExtractedText: the visible text, hidden text and invisible characters removed
// email.Screening: *Screening{Verdict, SenderTrust, QuarantineReason, InjectionScore, …}
// email.Extracted: *ReceivedEmailExtracted{Kinds, Code, Link, …} on verification mail
client.Emails.Receiving.Attachments.List(id, nil) // each with a signed DownloadUrl (1 hour)
client.Emails.Receiving.Attachments.Get(id, attachmentID)
client.Emails.Receiving.Remove(id) // erase now (full-access keys)

// Long-poll: wait up to 50 s for the next match
page, _ = client.Emails.Receiving.ListWithContext(ctx, &millionsend.ListReceivedEmailsOptions{
	InboxId: inboxID, From: "github.com", Wait: 30,
})
// page.Data: what arrived; page.Held: []HeldEmail{Id, InboxId, Reason, QuarantineReason, CreatedAt},
// matches held for screening or quarantine

// Reply and forward from the server, threaded
res, err := client.Emails.Receiving.Reply(id, &millionsend.ReplyReceivedEmailRequest{Text: "Thanks!"},
	millionsend.WithIdempotencyKey("reply-"+id))
if res.Draft != nil {
	// held for a person to approve (HTTP 202): res.Draft.ApprovalReasons says why
}
client.Emails.Receiving.Forward(id, &millionsend.ForwardReceivedEmailRequest{
	To: []string{"ops@acme.dev"}, Mode: "attachment", // the original as an .eml
})
```

`ListReceivedEmailsOptions` also takes `Before`/`After`, `Since`, `To`,
`SubjectContains`, `Unread` (with `InboxId`, except on an inbox-scoped key), and
`IncludeQuarantined` or `Quarantined` (full-access keys). `From` is an address,
or a domain covering its subdomains. An `InboxId` the key can't reach answers
`404 not_found` at once, even with `Wait`.
Replies and forwards return a `*SendOutcome[T]`: `Sent` once the email went
out, or `Draft` when an inbox's rules hold it for approval.

### Inboxes (beta)

Addresses whose mail lands in threads, for an agent (`Kind: "agent"`, the
default: screening, code extraction and send rules on) or for people
(`Kind: "human"`). Mirrors Resend's Inboxes beta; `Safety`, `Codes` and
`Quarantine` are MillionSend extensions.

```go
inbox, _ := client.Inboxes.Create(&millionsend.CreateInboxRequest{
	Name:      "Signup bot", // 1-320 characters
	Username:  "signup-bot", // signup-bot@<team>.<shared domain>; without it, a random name
	ExpiresIn: "24h",        // "1h" to "365d"; empty, it never expires
})
inbox, _ = client.Inboxes.Create(&millionsend.CreateInboxRequest{EmailAddress: "support@acme.dev", Kind: "human"})
client.Inboxes.Create(&millionsend.CreateInboxRequest{DomainId: domainID, Username: "billing"}) // billing@<your domain>, by its Domains.List id
client.Inboxes.List(&millionsend.ListInboxesOptions{Kind: "agent"})
client.Inboxes.Get("support@acme.dev") // by id or address
upd := &millionsend.UpdateInboxRequest{Name: "Support", FromName: millionsend.Ptr("Acme Support")}
upd.ClearExpiresIn() // never expires
client.Inboxes.Update(inbox.Id, upd)
client.Inboxes.Remove(inbox.Id) // also revokes its inbox-scoped keys

// Safety rules: who may write in and what an agent may send
client.Inboxes.Safety.Get(inbox.Id)
client.Inboxes.Safety.Update(inbox.Id, &millionsend.InboxSafetyUpdate{
	AcceptFrom: "allowlist", Allowlist: millionsend.Ptr([]string{"*@stripe.com"}),
})

// Verification codes and sign-in links (never fetched); waits up to 50 s
code, _ := client.Inboxes.Codes.Latest(inbox.Id, &millionsend.LatestCodeOptions{From: "github.com", Wait: 30})
// code.Status: "found" | "none" | "ambiguous" | "timeout"; code.Code ("482913") or code.Url
// A second sign-in: only mail received after the code already used counts
next, _ := client.Inboxes.Codes.Latest(inbox.Id, &millionsend.LatestCodeOptions{Wait: 30, Since: code.ReceivedAt})

// Quarantine: held mail is kept and listed; agents see it only once a person releases it
client.Inboxes.Quarantine.List(inbox.Id, nil)
client.Inboxes.Quarantine.Release(inbox.Id, emailID, &millionsend.ReleaseQuarantinedRequest{TrustSender: true})

// Threads and their messages
client.Inboxes.Threads.List(inbox.Id, &millionsend.ListInboxThreadsOptions{Folders: []string{"inbox", "archive"}, Read: millionsend.Ptr(false)})
client.Inboxes.Threads.List(inbox.Id, &millionsend.ListInboxThreadsOptions{IncludeQuarantined: true}) // full-access team keys: Data[i].Quarantined
client.Inboxes.Threads.Search(inbox.Id, &millionsend.SearchInboxThreadsOptions{Query: "invoice", From: []string{"stripe.com"}})
client.Inboxes.Threads.Get(inbox.Id, threadID)
client.Inboxes.Threads.Update(inbox.Id, threadID, &millionsend.UpdateInboxThreadRequest{Folder: "archive", LabelId: labelID})
client.Inboxes.Threads.Remove(inbox.Id, threadID) // to trash, purged after 30 days
client.Inboxes.Threads.Emails.List(inbox.Id, threadID, nil) // oldest first
client.Inboxes.Threads.Emails.Get(inbox.Id, threadID, emailID)
client.Inboxes.Threads.Emails.Reply(inbox.Id, threadID, emailID, &millionsend.ReplyInThreadRequest{Text: "On it."},
	millionsend.WithIdempotencyKey("k1"))
client.Inboxes.Threads.Emails.Forward(inbox.Id, threadID, emailID, &millionsend.ForwardInThreadRequest{To: []string{"ops@acme.dev"}})

// Labels
label, _ := client.Inboxes.Labels.Create(inbox.Id, &millionsend.CreateInboxLabelRequest{Name: "Billing", Color: "#FFAA00"})
client.Inboxes.Labels.List(inbox.Id)
client.Inboxes.Labels.Update(inbox.Id, label.Id, &millionsend.UpdateInboxLabelRequest{Name: "Invoices"})
client.Inboxes.Labels.Remove(inbox.Id, label.Id)

// Drafts
draft, _ := client.Inboxes.Drafts.Create(inbox.Id, &millionsend.CreateInboxDraftRequest{
	To: []string{"ada@x.dev"}, Subject: "Hello", Text: "Hi Ada",
})
client.Inboxes.Drafts.List(inbox.Id, nil)
client.Inboxes.Drafts.Get(inbox.Id, draft.Id)
client.Inboxes.Drafts.Update(inbox.Id, draft.Id, &millionsend.UpdateInboxDraftRequest{Text: "Hi Ada!"})
client.Inboxes.Drafts.Send(inbox.Id, draft.Id, millionsend.WithIdempotencyKey("draft-1"))
client.Inboxes.Drafts.Reject(inbox.Id, draft.Id) // approvers only
client.Inboxes.Drafts.Remove(inbox.Id, draft.Id)
```

Thread replies, forwards and `Drafts.Send` return a `*SendOutcome[T]` like
`Emails.Receiving.Reply`. An inbox-scoped key or an MCP token cannot send a
draft that waits for approval (`approval_required`); a person in the dashboard
or a full-access team key can. Slices in list and search options (`Folders`,
`Labels`, `From`, …) go out as one comma-separated value. Resend's hosted agent
settings (`/inboxes/:id/agent`) are not exposed: MillionSend runs no agent.

`Inboxes.Create` takes no Idempotency-Key. After a 503 or 429 nothing was
created, so sending it again is safe; after a timeout a retry may create a
second inbox, and with a chosen `Username` a 409 on the retry usually means
the first attempt worked (`Inboxes.Get(address)` finds it). Older servers
ignore fields they don't know: `Username` before v0.12.0 and `DomainId`
before v0.14.0, so check the created inbox's `EmailAddress`; `Since` before
v0.13.0, so the code you already used can come back.

#### The shared domain

Without `EmailAddress`, an inbox gets an address on the instance's shared
inbox domain, which needs no DNS: `<username>@<team>.<shared domain>`. That
subdomain is the `Shared` row of [`Domains.List`](#domains), whose `Id` is the
inbox's `DomainId`; passing it as `DomainId` works like leaving `EmailAddress`
out. With the id of one of your own domains instead, `Username` is required
and the address is `<username>@<domain>` (the domain needs receiving on, or
`Forwarding: true`).

- **`Username`** is trimmed and lowercased: 1-64 of `a-z 0-9 . _ -`, starting
  and ending with a letter or digit, no `..`. Reserved names (`postmaster`,
  `abuse`, `admin`, …), a `fwd-` prefix and brand look-alikes get 422, and a
  name in use 409. `Username` or `DomainId` together with `EmailAddress` is a
  422.
- **Name review.** A chosen `Username` or `FromName` (on create and update) is
  reviewed first. While the review is unavailable, the answer is `503
  name_review_unavailable` with `RetryAfter` 60 (leaving `Username` out still
  gives a random address). While a new team's subdomain waits for its own
  review, every shared-domain or `Forwarding` inbox create gets that 503 with
  `RetryAfter` 300. Past the daily review budget, the answer is `429
  rate_limit_exceeded` until the next UTC midnight.
- **Lifetime.** Inboxes never expire unless `ExpiresIn` says so. A
  shared-domain inbox unused for 90 days (no mail in or out, no API call on
  it) is deleted, firing `inbox.expired` and then `inbox.deleted`.
- **Sending** follows the row's `SendingMode`: `open` starts conversations
  (with [`Emails.Send`](#emails) too), `reply_only` only replies inside a
  thread to its own correspondents, and `receive_only` sends nothing. An inbox
  whose name looks like a brand only replies, even under `open`. Batch and
  broadcast sends never use the shared domain.
- **Safety.** A `SendPolicy` of `open` is accepted only while the mode is
  `open`, and `allowlist` never; `OwnerAddresses` don't make senders trusted;
  `ForwardTo` copies go only to team members' sign-in addresses.

The refusals are the same for `Emails.Send`, replies, forwards and
`Drafts.Send`:

| Status | Name | When |
|---|---|---|
| 403 | `inbox_send_policy` | `SendingMode` is `receive_only`, or `reply_only` outside a thread (`SendingModeReason` says whether the plan or the instance holds it); an inbox name that looks like a brand; any batch or broadcast send |
| 429 | `inbox_send_cap_reached` | the team's daily cap for mail from the shared domain; `RetryAfter` runs to the next UTC midnight |
| 422 | `validation_error` | more than 10 distinct recipients (`Drafts.Create` too); on `Emails.Send`, a `From` or field the shared domain doesn't take |

A person or a team key sending a draft skips the inbox's own rules, never the
shared domain's. An agent's reply to a `Reply-To` at another organization is
held: a `Draft` outcome whose `ApprovalReasons` include `reply_to_redirect`.

### Contacts

Contacts are team-global: one list per team, unique by email
(case-insensitive). Creating a duplicate is a `409 validation_error`.

```go
client.Contacts.Create(&millionsend.CreateContactRequest{
	Email: "ada@acme.dev", FirstName: "Ada",
	Properties: map[string]any{"plan": "pro"},
	Segments:   []millionsend.ContactSegmentRef{{Id: segmentID}},
	Topics:     []millionsend.ContactTopicUpdate{{Id: topicID, Subscription: "opt_in"}},
})
client.Contacts.Get(millionsend.ContactAddress{Email: "ada@acme.dev"}) // id or email (email wins)
client.Contacts.Update(&millionsend.UpdateContactRequest{
	Id: contactID, Unsubscribed: millionsend.Ptr(true), FirstName: millionsend.Ptr("Ada"),
}) // nil fields are left unchanged
client.Contacts.Remove(millionsend.ContactAddress{Email: "ada@acme.dev"}) // its emails stay in the send log
client.Contacts.Remove(millionsend.ContactAddress{Email: "ada@acme.dev"},
	millionsend.WithErase()) // also scrubs the address from email history, events and API logs (GDPR/LGPD)
client.Contacts.List(&millionsend.ListOptions{Limit: 50})
// Bulk read (MillionSend extension): attach the property map and the topic
// subscriptions to every item, so an audience reads in one request per 100
// contacts instead of one per contact. Segments.ListContacts takes it too.
client.Contacts.List(&millionsend.ListOptions{Limit: 100},
	millionsend.WithInclude(millionsend.ContactIncludeProperties, millionsend.ContactIncludeTopics))
// → Data[i].Properties (map[string]ContactPropertyValue) and Data[i].Topics ([]ContactTopic);
//   nil without the facet

// Topic subscriptions (granular unsubscribe)
client.Contacts.UpdateTopics(
	millionsend.ContactAddress{Email: "ada@acme.dev"},
	[]millionsend.ContactTopicUpdate{{Id: topicID, Subscription: "opt_out"}},
)
client.Contacts.Topics.List(millionsend.ContactAddress{Email: "ada@acme.dev"})
// → Data[i]{Id, Name, Description, Subscription: "opt_in" | "opt_out", Explicit, Visibility}
//   Subscription is the effective choice; Explicit is false when it is the topic's default

// Preference center (MillionSend extension): the hosted page the unsubscribe
// links open, for deep-linking from your own settings screen. No expiry and
// anyone holding it can change that contact's preferences, so hand it only to
// the contact. 422 when the instance cannot build hosted links.
link, _ := client.Contacts.CreatePreferencesLink(millionsend.ContactAddress{Email: "ada@acme.dev"})
// link.Url

// Segment membership
client.Contacts.Segments.Add(&millionsend.AddContactSegmentRequest{SegmentId: segmentID, ContactId: contactID})
client.Contacts.Segments.Remove(&millionsend.RemoveContactSegmentRequest{SegmentId: segmentID, Email: "ada@acme.dev"})
```

`Contact.Properties` is `map[string]ContactPropertyValue{Type, Value}` — the
typed `{type, value}` objects the API returns.

#### Bulk create, read and delete (MillionSend extension)

`POST /contacts/batch` writes up to 1000 contacts in one call. `WithOnConflict`
decides what happens to an email that already exists: `OnConflictError`
(default), `OnConflictSkip` (keep the existing contact, report its id) or
`OnConflictUpsert` (merge names/properties, add segments and topics). A batch
never re-subscribes anyone.

```go
res, err := client.Contacts.Batch.Create([]*millionsend.CreateContactRequest{
	{Email: "a@x.dev"}, {Email: "b@x.dev", FirstName: "B"},
}, millionsend.WithOnConflict(millionsend.OnConflictUpsert),
	millionsend.WithBatchValidation(millionsend.BatchValidationPermissive))
// res.Data[i]  → {Index, Id, Status: "created" | "updated" | "skipped"}
// res.Counts   → {Created, Updated, Skipped, Failed}
// res.Errors   → permissive mode only: {Index, StatusCode, Name, Message} per failed item
```

`POST /contacts/batch/get` reads up to 1000 contacts by id or email in one
call (one request against the rate limit), in request order. Entries that
match no contact are listed in `Missing` instead of failing the call;
`WithInclude` attaches `Properties` and/or `Topics` to each contact.

```go
res, err := client.Contacts.Batch.Get([]millionsend.ContactAddress{{Id: contactID}, {Email: "b@x.dev"}},
	millionsend.WithInclude(millionsend.ContactIncludeTopics))
// res.Data[i]    → {Object: "contact", Id, Email, FirstName, LastName, CreatedAt, Unsubscribed, Topics}
// res.Missing[i] → {Index, Id | Email}: request entries that matched nobody
```

`POST /contacts/batch/remove` deletes up to 1000 contacts by `Ids` or by
`Emails` (exactly one of the two; emails match case-insensitively) and lists
only the rows actually deleted. Each deletion is the same as `Contacts.Remove`:
the emails stay in the send log, and `Erase: true` also scrubs each address
from email history, event payloads and API logs.

```go
rm, err := client.Contacts.Batch.Remove(&millionsend.BatchRemoveContactsRequest{Emails: []string{"a@x.dev", "b@x.dev"}})
// rm.Data[i] → {Object: "contact", Contact: id, Deleted: true}
rm, err = client.Contacts.Batch.Remove(&millionsend.BatchRemoveContactsRequest{Ids: ids, Erase: true}) // GDPR/LGPD erasure
```

### Contact properties

The schema of the custom properties contacts carry. `Type` is `"string"` or
`"number"`; `FallbackValue` fills merge fields when a contact has no value.

```go
client.ContactProperties.Create(&millionsend.CreateContactPropertyRequest{Key: "plan", Type: "string", FallbackValue: "free"})
client.ContactProperties.List(nil)
client.ContactProperties.Get(id)
client.ContactProperties.Update(id, &millionsend.UpdateContactPropertyRequest{FallbackValue: "trial"}) // nil clears
client.ContactProperties.Remove(id)
```

### Topics

```go
client.Topics.Create(&millionsend.CreateTopicRequest{Name: "Product updates", DefaultSubscription: "opt_in", Visibility: "public"})
client.Topics.Get(id)
client.Topics.List()   // unpaginated: HasMore is always false
client.Topics.Update(id, &millionsend.UpdateTopicRequest{Description: "Monthly digest"})
client.Topics.Remove(id)
```

### Broadcasts

Targeting is an optional `SegmentId` and/or `TopicId`; set neither to send to
all the team's contacts. `Send: true` sends (or, with `ScheduledAt`, schedules)
on create instead of saving a draft. `Subject` takes up to 998 characters.
Broadcasts never send from [the shared domain](#the-shared-domain): `Create`
with `Send: true` and `Send` answer `403 inbox_send_policy`.

```go
b, _ := client.Broadcasts.Create(&millionsend.CreateBroadcastRequest{
	SegmentId: segmentID, From: "Acme <news@acme.dev>", Subject: "Launch",
	Html: "<p>Hi {{{FIRST_NAME|there}}}</p>", PreviewText: "It's here",
})
client.Broadcasts.List(nil)
client.Broadcasts.Get(b.Id)
client.Broadcasts.Update(b.Id, &millionsend.UpdateBroadcastRequest{Subject: "Launch 🚀"}) // draft only
client.Broadcasts.Send(b.Id, &millionsend.SendBroadcastRequest{ScheduledAt: "2026-09-01T09:00:00Z"}) // nil to send now
client.Broadcasts.Cancel(b.Id) // scheduled only
client.Broadcasts.Remove(b.Id) // draft only
```

### Suppressions

The team's do-not-send list. `Origin` is `bounce`, `complaint`, `manual`
(default) or `unsubscribe` (constants `SuppressionOrigin…`). Adding is
idempotent: an address already suppressed keeps its entry and origin.

```go
client.Suppressions.Add(&millionsend.AddSuppressionRequest{Email: "gone@x.dev"})
client.Suppressions.Get("gone@x.dev") // id or email
client.Suppressions.List(&millionsend.ListSuppressionsOptions{Origin: millionsend.SuppressionOriginBounce, Limit: 50})
client.Suppressions.Remove("gone@x.dev")

client.Suppressions.Batch.Add(&millionsend.BatchAddSuppressionsRequest{Emails: []string{"a@x.dev", "b@x.dev"}}) // up to 1000
client.Suppressions.Batch.Remove(&millionsend.BatchRemoveSuppressionsRequest{Emails: []string{"a@x.dev"}})     // or Ids
```

### Domains

```go
d, _ := client.Domains.Create(&millionsend.CreateDomainRequest{
	Name: "acme.dev", ClickTracking: millionsend.Ptr(true), TrackingSubdomain: "links",
})
for _, r := range d.Records { fmt.Println(r.Record, r.Type, r.Name, r.Value) } // DNS to publish
client.Domains.List(nil)
client.Domains.Get(d.Id)
client.Domains.Verify(d.Id) // re-checks DNS; returns per-record Status
client.Domains.Update(d.Id, &millionsend.UpdateDomainRequest{OpenTracking: millionsend.Ptr(true)})
client.Domains.Update(d.Id, &millionsend.UpdateDomainRequest{ReceivingCatchAll: millionsend.Ptr(true)}) // keep mail to addresses without an inbox (paid plans on Cloud)
client.Domains.Remove(d.Id)
```

`Region` must match the deployment's SES region — omit it to use the default.
`UpdateDomainRequest.Tls` is passed through unchanged: the API does not expose
a TLS policy and answers `422` rather than dropping it.

On an instance with a shared inbox domain, `List` ends with your team's
subdomain of it (`<team>.<shared domain>`, see
[the shared domain](#the-shared-domain)), after every page of your own
domains, so it can come alone on a last page. Every row carries `Shared`:
`true` there, `false` on your own domains and on every row from a server
before v0.14.0. Only the shared row has `SendingMode` (`open`, `reply_only` or
`receive_only`: what its inboxes may send) and `SendingModeReason` (`plan` or
`instance`, what holds it below `open`; `""` when `open`). Its
`Capabilities.Sending` is `disabled` under `receive_only`, and `Get` returns
it with no `Records`. It is read-only: `Update`, `Verify` and `Remove` answer
`403 forbidden`. A new team's row shows up once its subdomain passes its name
review.

The shared row sends only from an inbox's own address, so leave it out when
picking a sender domain:

```go
page, _ := client.Domains.List(nil)
for _, d := range page.Data {
	if d.Status == "verified" && !d.Shared {
		// a domain you can send from
	}
}
```

### Webhooks

Events: `email.sent`, `email.delivered`, `email.delivery_delayed`,
`email.bounced`, `email.complained`, `email.opened`, `email.clicked`,
`contact.created`, `contact.updated`, `contact.deleted`, plus the MillionSend
extensions `contact.unsubscribed`, `contact.resubscribed`,
`contact.topic_opt_in`, `contact.topic_opt_out`, `suppression.added`,
`suppression.removed`, `deliverability.warning`, `deliverability.paused`,
`quota.warning`, `quota.reached`, `quota.paused`, `email.received`, and the
inbox events `inbox.created`, `inbox.updated`, `inbox.deleted`,
`inbox.expired`, `inbox.thread.created`, `inbox.thread.folder.updated`,
`inbox.thread.labels.updated`, `inbox.email.received`, `inbox.email.sent`,
`inbox.email.quarantined`, `inbox.draft.created`, `inbox.draft.updated`,
`inbox.draft.sent`, `inbox.draft.deleted`. Inbox events are opt-in: an
endpoint gets them only when its `Events` name them, and `InboxIds` limits
them (and `email.received`) to those inboxes. Their `data.source` says who
acted: `api`, `dashboard` or `system`. A shared-domain inbox deleted after 90
unused days fires `inbox.expired`, then `inbox.deleted`.

```go
w, _ := client.Webhooks.Create(&millionsend.CreateWebhookRequest{
	Endpoint: "https://acme.dev/hooks/millionsend",
	Events:   []string{"email.delivered", "email.bounced"},
}) // w.SigningSecret
client.Webhooks.List(nil)           // rows never carry the secret
client.Webhooks.Get(w.Id)           // includes SigningSecret and PreviousSecretExpiresAt
client.Webhooks.Update(w.Id, &millionsend.UpdateWebhookRequest{Status: "disabled"})
client.Webhooks.Update(w.Id, &millionsend.UpdateWebhookRequest{InboxIds: []string{inboxID}}) // ClearInboxIds(): every inbox
client.Webhooks.Remove(w.Id)

// Rotate the secret (MillionSend extension). For OverlapHours (default 24,
// up to 72) every delivery carries both signatures, so the receiver can
// switch at any point in the window; Ptr(0) drops the old secret at once.
r, _ := client.Webhooks.Rotate(w.Id, &millionsend.RotateWebhookSecretRequest{OverlapHours: millionsend.Ptr(48)})
// r.SigningSecret, r.PreviousSecretExpiresAt (*string; nil once the old secret is gone)
```

Pass `SigningSecret` on create (or rotate) to carry over an existing `whsec_…`
secret.

### API keys

```go
k, _ := client.ApiKeys.Create(&millionsend.CreateApiKeyRequest{
	Name: "ci", Permission: "sending_access", DomainId: domainID, // one of your domains: a Shared false row
}) // k.Token is returned only here
client.ApiKeys.Create(&millionsend.CreateApiKeyRequest{
	Name: "signup-bot", Permission: "full_access", InboxId: inboxID,
}) // reads and sends for that one inbox only; list rows carry InboxId
client.ApiKeys.List(nil)
client.ApiKeys.Remove(k.Id)
```

### Templates

Every method takes the template id or its alias. Templates have no
draft/publish cycle — every save is live — so `Publish` is a no-op kept for
resend-go compatibility.

```go
t, _ := client.Templates.Create(&millionsend.CreateTemplateRequest{
	Name: "Welcome", Alias: "welcome", Subject: "Hi {{{FIRST_NAME|there}}}", Html: "<p>Welcome</p>",
})
client.Templates.List(nil)
client.Templates.Get("welcome")
client.Templates.Update("welcome", &millionsend.UpdateTemplateRequest{Subject: "Hello"})
client.Templates.Duplicate(t.Id) // "<name> (copy)", no alias
client.Templates.Publish(t.Id)   // no-op
client.Templates.Remove(t.Id)
```

`From`, `ReplyTo` and `Variables` are accepted for resend-go compatibility and
passed through unchanged; the API does not support them yet and answers `422`
(they read back as `""`, `nil` and empty).

### Segments (MillionSend extension)

Segments are a saved filter over the team's contacts — a MillionSend superset
with no Resend equivalent (served at `/segments`). Omit `Filter` to create a
manual segment, whose members come only from `Contacts.Segments.Add` and
`CreateContactRequest.Segments`.

```go
client.Segments.Create(&millionsend.CreateSegmentRequest{
	Name: "Pro plan",
	Filter: millionsend.SegmentFilter{
		Match: "all",
		Conditions: []millionsend.SegmentCondition{
			{Field: "property:plan", Op: "equals", Value: "pro"},
			{Field: "first_name", Op: "is_set"}, // presence ops ignore Value
		},
	},
})
client.Segments.Create(&millionsend.CreateSegmentRequest{Name: "VIPs"}) // manual segment
client.Segments.Get(id) // includes a live ContactCount
client.Segments.List(nil)
client.Segments.ListContacts(id, &millionsend.ListOptions{Limit: 100}) // takes WithInclude like Contacts.List
client.Segments.Update(id, &millionsend.UpdateSegmentRequest{Name: "Pro tier"})
client.Segments.Remove(id)
```

`UpdateSegmentRequest.ClearFilter()` sends `filter: null`, turning a filtered
segment into a manual one.

### Deliverability (MillionSend extension)

The account-level deliverability score over the trailing window. Nullable
scores (`Score`, `Band`, `ContentScore`, `OutcomeScore`) are pointers — nil
means not enough data yet. Band, check severity/status and guardrail status
are plain strings: new values may appear without an SDK update.

```go
d, _ := client.Deliverability.Get()
if d.Score != nil {
	fmt.Printf("%.1f (%s)\n", *d.Score, *d.Band)
}
```

### Usage (MillionSend extension)

The effective plan, its limits and today's accepted send count. `Plan` and the
`Limits` are nil on a self-hosted instance.

```go
u, _ := client.Usage.Get()
fmt.Println(u.Today.EmailsSent, "sent today; resets", u.Today.ResetsAt)
```

## Server versions

On a self-hosted instance, each feature needs at least this MillionSend version:

| Server | Features |
|---|---|
| v0.8.0 | `Emails.Receiving` |
| v0.10.0 | `Inboxes` |
| v0.11.0 | `422` for recipients that can never receive mail; a retried batch gets its stored response |
| v0.12.0 | `Username`; inboxes that never expire by default; sends from shared-domain inboxes; `name_review_unavailable` and the daily name-review budget; `StatusCode`/`Name` on contacts-batch errors |
| v0.13.0 | Newest-first lists; `Since` on `Codes.Latest`; `Quarantined` on thread items; inbox names up to 320 characters; `404` at once for an unknown `InboxId`; `Unread` on inbox-scoped keys; JSON `404` for unknown routes |
| v0.14.0 | The shared row in `Domains.List` (`Shared`, `SendingMode`, `SendingModeReason`); `DomainId` on `Inboxes.Create`; `Subject` up to 998 characters |

## Migrating from Resend

```diff
- import "github.com/resend/resend-go/v4"
- client := resend.NewClient("re_123")
+ import millionsend "github.com/MillionSend/millionsend-go"
+ client := millionsend.NewClient("ms_123")
+ client.BaseURL = "https://mail.acme.dev" // self-hosted only
```

Method names and nesting match (`client.Emails.Send`, `client.Contacts`,
`client.Broadcasts`, `client.Domains`, `client.ApiKeys`, `client.Webhooks`,
`client.Templates`, `client.Suppressions.Batch`, …). Notes:

- **No audiences**: MillionSend contacts are one team-global list. Drop the
  audience id from `.Contacts` calls; target broadcasts with a `.Segments`
  filter (or a topic) instead. The API keeps `/audiences/*` only as a
  compatibility shim for raw HTTP clients; it is not part of this SDK.
- **Pagination** is `*ListOptions{Limit, After, Before}` (plain values, not
  pointers) passed to `List`.
- **Contacts are addressed** with `ContactAddress{Id | Email}` instead of
  per-call option structs.
- **Template sends** (`SendEmailRequest.Template`) are passed through but
  answered with `422`: the API does not render stored templates yet.
- **Received mail** is `client.Emails.Receiving` as in resend-go (`Get`,
  `GetWithOptions`, `List`), but attachments hang off
  `Emails.Receiving.Attachments.List/Get` and one `ReceivedEmail` type serves
  list rows and full reads.
- **Extensions with no Resend counterpart**: `Segments`, `Contacts.Batch`,
  `Contacts.CreatePreferencesLink`, `Webhooks.Rotate`, `Deliverability`,
  `Usage`, `Emails.GetInsights`, the `unsubscribe` suppression origin, the
  receiving long-poll, `Emails.Receiving.Reply/Forward/Remove`,
  `Inboxes.Safety/Codes/Quarantine`, and the `deliverability.*` / `quota.*` /
  `suppression.*` webhook events (plus the `contact.*` events beyond Resend's
  created/updated/deleted).

## License

MIT
