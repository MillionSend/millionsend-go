# millionsend-go

Official Go client for [MillionSend](https://github.com/MillionSend/millionsend) — a
self-hostable, Resend-compatible email API on AWS SES.

The API is wire-compatible with Resend, and this SDK mirrors the shape of
[`resend-go`](https://github.com/resend/resend-go), so migrating is mostly a
find-and-replace: swap the import and constructor, then point `BaseURL` at your
instance.

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
	client.BaseURL = "https://mail.acme.dev" // self-hosted: set this in production

	sent, err := client.Emails.Send(&millionsend.SendEmailRequest{
		From:    "Acme <onboarding@acme.dev>",
		To:      []string{"delivered@resend.dev"},
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
client.BaseURL = "https://mail.acme.dev"        // override the base URL
client.HTTPClient = &http.Client{Timeout: time.Minute} // bring your own transport
client.UserAgent = "myapp/1.0 " + client.UserAgent      // extend the User-Agent
```

- `apiKey` falls back to `MILLIONSEND_API_KEY` when empty.
- `BaseURL` falls back to `MILLIONSEND_BASE_URL`, then `http://localhost:3001`.
  MillionSend is self-hosted, so **set this to your deployment in production.**

Every method has a non-context form and, for the transactional hot path,
`Emails.Send`/`Emails.Get`/`Emails.Cancel` and `Batch.Send` also expose a
`…WithContext(ctx, …)` variant.

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
		}
		log.Printf("status=%d name=%s: %s", apiErr.StatusCode, apiErr.Name, apiErr.Message)
	}
}
```

`StatusCode` is `0` when the request never reached the API (a transport or
client-side failure) — the wire's `statusCode: null`.

## Resources

### Emails

```go
client.Emails.Send(&millionsend.SendEmailRequest{...}, millionsend.WithIdempotencyKey("key"))
client.Emails.Get(id)
client.Emails.Cancel(id) // scheduled, unsent emails only

client.Batch.Send([]*millionsend.SendEmailRequest{a, b}, millionsend.WithIdempotencyKey("key")) // up to 100
```

`To`/`Cc`/`Bcc` are `[]string`; `ReplyTo` and `ScheduledAt` map to `reply_to`
and `scheduled_at` on the wire.

### Audiences & contacts

```go
aud, _ := client.Audiences.Create(&millionsend.CreateAudienceRequest{Name: "Registered users"})
client.Audiences.List(&millionsend.ListOptions{Limit: 20, After: cursor})
client.Audiences.Get(aud.Id)
client.Audiences.Remove(aud.Id)

client.Contacts.Create(&millionsend.CreateContactRequest{
	AudienceId: aud.Id, Email: "ada@acme.dev", FirstName: "Ada",
	Properties: map[string]any{"plan": "pro"},
})
client.Contacts.Get(millionsend.ContactAddress{AudienceId: aud.Id, Email: "ada@acme.dev"}) // id or email (email wins)
client.Contacts.Update(&millionsend.UpdateContactRequest{
	Id: contactID, Unsubscribed: millionsend.Ptr(true), FirstName: millionsend.Ptr("Ada"),
}) // nil fields are left unchanged
client.Contacts.Remove(millionsend.ContactAddress{Email: "ada@acme.dev"})
client.Contacts.List(aud.Id, &millionsend.ListOptions{Limit: 50}) // "" for the top-level list

// Topic subscriptions (granular unsubscribe)
client.Contacts.UpdateTopics(
	millionsend.ContactAddress{Email: "ada@acme.dev"},
	[]millionsend.ContactTopicUpdate{{Id: topicID, Subscription: "opt_out"}},
)
```

### Topics

```go
client.Topics.Create(&millionsend.CreateTopicRequest{Name: "Product updates", DefaultSubscription: "opt_in"})
client.Topics.Get(id)
client.Topics.List()   // bare { data } — topics are unpaginated
client.Topics.Remove(id)
```

### Broadcasts

```go
b, _ := client.Broadcasts.Create(&millionsend.CreateBroadcastRequest{
	AudienceId: aud.Id, From: "Acme <news@acme.dev>", Subject: "Launch",
	Html: "<p>Hi {{{FIRST_NAME|there}}}</p>",
})
client.Broadcasts.List(nil)
client.Broadcasts.Get(b.Id)
client.Broadcasts.Update(b.Id, &millionsend.UpdateBroadcastRequest{Subject: "Launch 🚀"}) // draft only
client.Broadcasts.Send(b.Id, &millionsend.SendBroadcastRequest{ScheduledAt: "2026-09-01T09:00:00Z"}) // nil to send now
client.Broadcasts.Cancel(b.Id) // scheduled only
client.Broadcasts.Remove(b.Id) // draft only
```

### Segments (MillionSend extension)

Dynamic segments are a saved filter over an audience's contacts — a MillionSend
superset with no Resend equivalent (served at `/segments2`).

```go
client.Segments.Create(&millionsend.CreateSegmentRequest{
	Name: "Pro plan", AudienceId: aud.Id,
	Filter: millionsend.SegmentFilter{
		Match:      "all",
		Conditions: []millionsend.SegmentCondition{{Field: "property:plan", Op: "equals", Value: "pro"}},
	},
})
client.Segments.Get(id) // includes a live ContactCount
client.Segments.List(nil)
client.Segments.Update(id, &millionsend.UpdateSegmentRequest{Name: "Pro tier"})
client.Segments.Remove(id)
```

## Migrating from Resend

```diff
- import "github.com/resend/resend-go/v2"
- client := resend.NewClient("re_123")
+ import millionsend "github.com/MillionSend/millionsend-go"
+ client := millionsend.NewClient("ms_123")
+ client.BaseURL = "https://mail.acme.dev"
```

Method names and nesting match (`client.Emails.Send`, `client.Audiences`,
`client.Contacts`, `client.Broadcasts`, …). Notes:

- **Domains and API keys** are managed in the MillionSend dashboard, not via the
  API, so there are no `.Domains`/`.ApiKeys` resources here.
- Resend's `.segments` is an alias of audiences; MillionSend's `.Segments` is the
  distinct dynamic-filter feature. Use `.Audiences` for a straight port.

## License

MIT
