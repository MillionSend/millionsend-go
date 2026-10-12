package millionsend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A body that decodes into every response type used below (unknown fields are
// ignored, lists get an empty data array).
const anyOK = `{"object":"x","id":"1","data":[],"has_more":false,"deleted":true}`

func TestUserAgentCarriesVersion(t *testing.T) {
	c, rec := mockServer(t, 200, anyOK)
	_, err := c.Usage.Get()
	require.NoError(t, err)
	assert.Equal(t, "millionsend-go/0.9.0", rec.Header.Get("User-Agent"))
}

func TestSendFullWireBody(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"abc"}`)
	_, err := c.Emails.Send(&SendEmailRequest{
		From:        "Acme <a@x.dev>",
		To:          []string{"b@x.dev", "c@x.dev"},
		Subject:     "s",
		Html:        "<p>h</p>",
		Text:        "t",
		Cc:          []string{"cc@x.dev"},
		Bcc:         []string{"bcc@x.dev"},
		ReplyTo:     "r@x.dev",
		ScheduledAt: "2999-01-01T00:00:00Z",
		Tags:        []Tag{{Name: "k", Value: "v"}},
		TopicId:     "11111111-1111-4111-8111-111111111111",
		Attachments: []*Attachment{{
			Filename:    "hi.txt",
			Content:     []byte("hello"),
			ContentType: "text/plain",
			ContentId:   "cid1",
			Path:        "https://x.dev/hi.txt",
		}},
		Headers:  map[string]string{"X-Entity-Ref-ID": "123"},
		Template: &EmailTemplate{Id: "welcome", Variables: map[string]any{"NAME": "Ada"}},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"from":"Acme <a@x.dev>",
		"to":["b@x.dev","c@x.dev"],
		"subject":"s",
		"html":"<p>h</p>",
		"text":"t",
		"cc":["cc@x.dev"],
		"bcc":["bcc@x.dev"],
		"reply_to":"r@x.dev",
		"scheduled_at":"2999-01-01T00:00:00Z",
		"tags":[{"name":"k","value":"v"}],
		"topic_id":"11111111-1111-4111-8111-111111111111",
		"attachments":[{"filename":"hi.txt","content":"aGVsbG8=","content_type":"text/plain","content_id":"cid1","path":"https://x.dev/hi.txt"}],
		"headers":{"X-Entity-Ref-ID":"123"},
		"template":{"id":"welcome","variables":{"NAME":"Ada"}}
	}`, string(rec.Body))
}

func TestSendWithOptionsResendShape(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"abc"}`)
	_, err := c.Emails.SendWithOptions(context.Background(), sampleEmail(), &SendEmailOptions{IdempotencyKey: "k-1"})
	require.NoError(t, err)
	assert.Equal(t, "k-1", rec.Header.Get("Idempotency-Key"))

	_, err = c.Emails.SendWithOptions(context.Background(), sampleEmail(), nil)
	require.NoError(t, err)
	assert.Empty(t, rec.Header.Get("Idempotency-Key"))
}

func TestBatchValidationHeaderAndErrors(t *testing.T) {
	c, rec := mockServer(t, 200, `{"data":[{"id":"1"}],"errors":[{"index":1,"message":"emails.1: invalid to"}]}`)
	res, err := c.Batch.SendWithOptions(context.Background(), []*SendEmailRequest{sampleEmail(), sampleEmail()},
		&BatchSendEmailOptions{IdempotencyKey: "b-1", BatchValidation: BatchValidationPermissive})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/emails/batch", rec.Path)
	assert.Equal(t, "permissive", rec.Header.Get("x-batch-validation"))
	assert.Equal(t, "b-1", rec.Header.Get("Idempotency-Key"))
	require.Len(t, res.Data, 1)
	require.Len(t, res.Errors, 1)
	assert.Equal(t, 1, res.Errors[0].Index)
	assert.Equal(t, "emails.1: invalid to", res.Errors[0].Message)

	// Functional-option form.
	_, err = c.Batch.Send([]*SendEmailRequest{sampleEmail()}, WithBatchValidation(BatchValidationStrict))
	require.NoError(t, err)
	assert.Equal(t, "strict", rec.Header.Get("x-batch-validation"))

	// No header unless asked (the server default is strict).
	_, err = c.Batch.Send([]*SendEmailRequest{sampleEmail()})
	require.NoError(t, err)
	assert.Empty(t, rec.Header.Get("x-batch-validation"))
}

func TestEmailsListUpdateRemove(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","data":[{"id":"e1","to":["b@x.dev"],"cc":null,"scheduled_at":null}],"has_more":false}`)
	list, err := c.Emails.List(&ListOptions{Limit: 5, After: "cur"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/emails", rec.Path)
	assert.Equal(t, "after=cur&limit=5", rec.RawQuery)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "e1", list.Data[0].Id)
	assert.Nil(t, list.Data[0].Cc)

	c, rec = mockServer(t, 200, `{"object":"email","id":"e1"}`)
	_, err = c.Emails.Update(&UpdateEmailRequest{Id: "e1", ScheduledAt: "2999-01-01T00:00:00Z"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/emails/e1", rec.Path)
	assert.JSONEq(t, `{"scheduled_at":"2999-01-01T00:00:00Z"}`, string(rec.Body))

	rm, err := c.Emails.Remove("e1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/emails/e1", rec.Path)
	assert.Equal(t, "e1", rm.Id)
}

func TestContactsCreateFullBody(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"contact","id":"c1"}`)
	_, err := c.Contacts.Create(&CreateContactRequest{
		Email:        "ada@x.dev",
		FirstName:    "Ada",
		LastName:     "Lovelace",
		Unsubscribed: true,
		Properties:   map[string]any{"plan": "pro", "seats": 3},
		Segments:     []ContactSegmentRef{{Id: "s1"}},
		Topics:       []ContactTopicUpdate{{Id: "t1", Subscription: "opt_out"}},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"email":"ada@x.dev","first_name":"Ada","last_name":"Lovelace","unsubscribed":true,
		"properties":{"plan":"pro","seats":3},
		"segments":[{"id":"s1"}],
		"topics":[{"id":"t1","subscription":"opt_out"}]
	}`, string(rec.Body))
}

func TestContactsUpdateClearsWithNull(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"contact","id":"c1"}`)
	req := &UpdateContactRequest{Id: "c1", FirstName: Ptr("Ada"), Properties: map[string]any{"plan": nil}}
	req.ClearLastName()
	_, err := c.Contacts.Update(req)
	require.NoError(t, err)
	assert.Equal(t, "/contacts/c1", rec.Path)
	assert.JSONEq(t, `{"first_name":"Ada","last_name":null,"properties":{"plan":null}}`, string(rec.Body))

	// Clearing without any other field still yields a body with only the null.
	req = &UpdateContactRequest{Email: "ada@x.dev"}
	req.ClearFirstName()
	_, err = c.Contacts.Update(req)
	require.NoError(t, err)
	assert.JSONEq(t, `{"first_name":null}`, string(rec.Body))
}

func TestContactGetDecodesTypedProperties(t *testing.T) {
	c, _ := mockServer(t, 200, `{"object":"contact","id":"c1","email":"ada@x.dev","first_name":null,"last_name":null,
		"created_at":"2026-01-01T00:00:00Z","unsubscribed":false,
		"properties":{"plan":{"type":"string","value":"pro"},"seats":{"type":"number","value":3}}}`)
	got, err := c.Contacts.Get(ContactAddress{Id: "c1"})
	require.NoError(t, err)
	assert.Equal(t, "", got.FirstName)
	require.Len(t, got.Properties, 2)
	assert.Equal(t, ContactPropertyValue{Type: "string", Value: "pro"}, got.Properties["plan"])
	assert.Equal(t, ContactPropertyValue{Type: "number", Value: float64(3)}, got.Properties["seats"])
}

func TestContactsBatchCreate(t *testing.T) {
	c, rec := mockServer(t, 200, `{
		"data":[{"object":"contact","index":0,"id":"c1","status":"created"},{"object":"contact","index":2,"id":"c2","status":"updated"}],
		"counts":{"created":1,"updated":1,"skipped":0,"failed":1},
		"errors":[{"index":1,"message":"contacts.1: invalid email"}]}`)
	res, err := c.Contacts.Batch.Create([]*CreateContactRequest{
		{Email: "a@x.dev"}, {Email: "nope"}, {Email: "b@x.dev", FirstName: "B"},
	}, WithOnConflict(OnConflictUpsert), WithBatchValidation(BatchValidationPermissive))
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/contacts/batch", rec.Path)
	assert.Equal(t, "on_conflict=upsert", rec.RawQuery)
	assert.Equal(t, "permissive", rec.Header.Get("x-batch-validation"))
	assert.JSONEq(t, `[{"email":"a@x.dev"},{"email":"nope"},{"email":"b@x.dev","first_name":"B"}]`, string(rec.Body))
	require.Len(t, res.Data, 2)
	assert.Equal(t, BatchContactResult{Object: "contact", Index: 2, Id: "c2", Status: "updated"}, res.Data[1])
	assert.Equal(t, BatchContactsCounts{Created: 1, Updated: 1, Skipped: 0, Failed: 1}, res.Counts)
	require.Len(t, res.Errors, 1)
	assert.Equal(t, BatchError{Index: 1, Message: "contacts.1: invalid email"}, res.Errors[0])

	// Defaults: no query, no header.
	_, err = c.Contacts.Batch.CreateWithContext(context.Background(), []*CreateContactRequest{{Email: "a@x.dev"}})
	require.NoError(t, err)
	assert.Empty(t, rec.RawQuery)
	assert.Empty(t, rec.Header.Get("x-batch-validation"))
}

func TestContactsBatchRemove(t *testing.T) {
	c, rec := mockServer(t, 200, `{"data":[{"object":"contact","contact":"c1","deleted":true}]}`)
	rm, err := c.Contacts.Batch.Remove(&BatchRemoveContactsRequest{Ids: []string{"c1", "c2"}})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/contacts/batch/remove", rec.Path)
	assert.JSONEq(t, `{"ids":["c1","c2"]}`, string(rec.Body))
	require.Len(t, rm.Data, 1)
	assert.Equal(t, RemoveContactResponse{Object: "contact", Contact: "c1", Deleted: true}, rm.Data[0])

	_, err = c.Contacts.Batch.RemoveWithContext(context.Background(), &BatchRemoveContactsRequest{Emails: []string{"a@x.dev"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"emails":["a@x.dev"]}`, string(rec.Body))

	_, err = c.Contacts.Batch.Remove(&BatchRemoveContactsRequest{Ids: []string{"c1"}, Erase: true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ids":["c1"],"erase":true}`, string(rec.Body))
}

func TestContactsListInclude(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","has_more":false,"data":[
		{"id":"c1","email":"a@x.dev","first_name":"A","last_name":"","created_at":"2026-01-01T00:00:00Z","unsubscribed":false,
		 "properties":{"plan":{"type":"string","value":"pro"},"seats":{"type":"number","value":3}},
		 "topics":[{"id":"t1","name":"News","description":null,"subscription":"opt_in","explicit":false,"visibility":"public"}]}]}`)
	res, err := c.Contacts.List(&ListOptions{Limit: 100}, WithInclude(ContactIncludeProperties, ContactIncludeTopics))
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/contacts", rec.Path)
	assert.Equal(t, "include=properties%2Ctopics&limit=100", rec.RawQuery)
	require.Len(t, res.Data, 1)
	assert.Equal(t, map[string]ContactPropertyValue{
		"plan":  {Type: "string", Value: "pro"},
		"seats": {Type: "number", Value: float64(3)},
	}, res.Data[0].Properties)
	assert.Equal(t, []ContactTopic{{Id: "t1", Name: "News", Subscription: "opt_in", Visibility: "public"}}, res.Data[0].Topics)

	// The facet rides alone when there are no pagination options.
	_, err = c.Contacts.List(nil, WithInclude(ContactIncludeTopics))
	require.NoError(t, err)
	assert.Equal(t, "include=topics", rec.RawQuery)

	// Without WithInclude the query is the plain pagination.
	_, err = c.Contacts.List(&ListOptions{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, "limit=100", rec.RawQuery)
}

func TestContactsBatchGet(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","data":[
		{"object":"contact","id":"c1","email":"a@x.dev","first_name":"A","last_name":"","created_at":"2026-01-01T00:00:00Z","unsubscribed":false,
		 "properties":{"plan":{"type":"string","value":"pro"}},"topics":[]}],
		"missing":[{"index":1,"email":"b@x.dev"},{"index":2,"id":"c3"}]}`)
	res, err := c.Contacts.Batch.Get([]ContactAddress{{Id: "c1"}, {Email: "b@x.dev"}, {Id: "c3"}},
		WithInclude(ContactIncludeProperties, ContactIncludeTopics))
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/contacts/batch/get", rec.Path)
	assert.Empty(t, rec.RawQuery)
	assert.JSONEq(t, `{"contacts":[{"id":"c1"},{"email":"b@x.dev"},{"id":"c3"}],"include":["properties","topics"]}`, string(rec.Body))
	assert.Equal(t, "list", res.Object)
	require.Len(t, res.Data, 1)
	assert.Equal(t, BatchGetContact{Object: "contact", ContactListItem: ContactListItem{
		Id: "c1", Email: "a@x.dev", FirstName: "A", CreatedAt: "2026-01-01T00:00:00Z",
		Properties: map[string]ContactPropertyValue{"plan": {Type: "string", Value: "pro"}},
		Topics:     []ContactTopic{},
	}}, res.Data[0])
	assert.Equal(t, []BatchGetMissingContact{{Index: 1, Email: "b@x.dev"}, {Index: 2, Id: "c3"}}, res.Missing)

	// Email wins over Id in an address; without WithInclude no include key is sent.
	_, err = c.Contacts.Batch.GetWithContext(context.Background(), []ContactAddress{{Id: "c2", Email: "c@x.dev"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"contacts":[{"email":"c@x.dev"}]}`, string(rec.Body))
}

func TestContactsCreatePreferencesLink(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"preferences_link","contact":"c1","url":"https://app.x.dev/p/tok"}`)
	link, err := c.Contacts.CreatePreferencesLink(ContactAddress{Email: "ada@x.dev"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/contacts/ada@x.dev/preferences-link", rec.Path)
	assert.Empty(t, rec.Body)
	assert.Equal(t, ContactPreferencesLink{Object: "preferences_link", Contact: "c1", Url: "https://app.x.dev/p/tok"}, *link)

	_, err = c.Contacts.CreatePreferencesLink(ContactAddress{Id: "c1"})
	require.NoError(t, err)
	assert.Equal(t, "/contacts/c1/preferences-link", rec.Path)

	c, _ = mockServer(t, 422, `{"statusCode":422,"name":"validation_error","message":"APP_BASE_URL and MASTER_ENCRYPTION_KEY must be set to mint preference links"}`)
	_, err = c.Contacts.CreatePreferencesLink(ContactAddress{Id: "c1"})
	var mse *MillionSendError
	require.ErrorAs(t, err, &mse)
	assert.Equal(t, 422, mse.StatusCode)
	assert.Equal(t, "validation_error", mse.Name)
}

func TestContactsSegmentsAddRemove(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"c1","audienceId":"s1","deleted":true}`)
	add, err := c.Contacts.Segments.Add(&AddContactSegmentRequest{SegmentId: "s1", ContactId: "c1"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/contacts/c1/segments/s1", rec.Path)
	assert.Empty(t, rec.Body)
	assert.Equal(t, "c1", add.Id)

	rm, err := c.Contacts.Segments.Remove(&RemoveContactSegmentRequest{SegmentId: "s1", Email: "ada@x.dev"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/contacts/ada@x.dev/segments/s1", rec.Path)
	assert.Equal(t, "s1", rm.AudienceId)
	assert.True(t, rm.Deleted)
}

func TestBroadcastsFullBodyAndClearTopic(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"b1"}`)
	_, err := c.Broadcasts.Create(&CreateBroadcastRequest{
		Name: "Launch", SegmentId: "s1", From: "Acme <news@x.dev>", Subject: "Hi",
		Html: "<p>h</p>", Text: "t", ReplyTo: "r@x.dev", PreviewText: "pre", TopicId: "t1",
		Send: true, ScheduledAt: "in 1 hour",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Launch","segment_id":"s1","from":"Acme <news@x.dev>","subject":"Hi","html":"<p>h</p>","text":"t",
		"reply_to":"r@x.dev","preview_text":"pre","topic_id":"t1","send":true,"scheduled_at":"in 1 hour"}`, string(rec.Body))

	upd := &UpdateBroadcastRequest{Subject: "New", PreviewText: "p2"}
	upd.ClearTopicId()
	_, err = c.Broadcasts.Update("b1", upd)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.JSONEq(t, `{"subject":"New","preview_text":"p2","topic_id":null}`, string(rec.Body))

	// Without Clear, topic_id is simply omitted.
	_, err = c.Broadcasts.Update("b1", &UpdateBroadcastRequest{TopicId: "t2"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"topic_id":"t2"}`, string(rec.Body))
}

func TestTopicsVisibilityAndUpdate(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"t1"}`)
	_, err := c.Topics.Create(&CreateTopicRequest{Name: "News", DefaultSubscription: "opt_in", Visibility: "public"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"News","default_subscription":"opt_in","visibility":"public"}`, string(rec.Body))

	_, err = c.Topics.Update("t1", &UpdateTopicRequest{Description: "d", Visibility: "private"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/topics/t1", rec.Path)
	assert.JSONEq(t, `{"description":"d","visibility":"private"}`, string(rec.Body))
}

func TestSegmentsListContacts(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","data":[{"id":"c1","email":"a@x.dev"}],"has_more":false}`)
	res, err := c.Segments.ListContacts("s1", &ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/segments/s1/contacts", rec.Path)
	assert.Equal(t, "limit=10", rec.RawQuery)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "a@x.dev", res.Data[0].Email)

	_, err = c.Segments.ListContacts("s1", nil, WithInclude(ContactIncludeTopics))
	require.NoError(t, err)
	assert.Equal(t, "/segments/s1/contacts", rec.Path)
	assert.Equal(t, "include=topics", rec.RawQuery)
}

func TestSuppressions(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"suppression","id":"s1","email":"a@x.dev","origin":"bounce","source_id":null,"created_at":"2026-01-01T00:00:00Z"}`)
	_, err := c.Suppressions.Add(&AddSuppressionRequest{Email: "a@x.dev", Origin: SuppressionOriginUnsubscribe})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/suppressions", rec.Path)
	assert.JSONEq(t, `{"email":"a@x.dev","origin":"unsubscribe"}`, string(rec.Body))

	_, err = c.Suppressions.Add(&AddSuppressionRequest{Email: "a@x.dev"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"email":"a@x.dev"}`, string(rec.Body))

	got, err := c.Suppressions.Get("a@x.dev")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/suppressions/a@x.dev", rec.Path)
	assert.Equal(t, "bounce", got.Origin)
	assert.Nil(t, got.SourceId)

	_, err = c.Suppressions.Remove("s1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/suppressions/s1", rec.Path)

	c, rec = mockServer(t, 200, `{"object":"list","data":[],"has_more":false}`)
	_, err = c.Suppressions.List(&ListSuppressionsOptions{Origin: SuppressionOriginComplaint, Limit: 5, After: "cur"})
	require.NoError(t, err)
	assert.Equal(t, "/suppressions", rec.Path)
	assert.Equal(t, "after=cur&limit=5&origin=complaint", rec.RawQuery)

	_, err = c.Suppressions.List(nil)
	require.NoError(t, err)
	assert.Empty(t, rec.RawQuery)
}

func TestSuppressionsBatch(t *testing.T) {
	c, rec := mockServer(t, 200, `{"data":[{"object":"suppression","id":"s1"},{"object":"suppression","id":"s2"}]}`)
	res, err := c.Suppressions.Batch.Add(&BatchAddSuppressionsRequest{Emails: []string{"a@x.dev", "b@x.dev"}, Origin: SuppressionOriginManual})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/suppressions/batch/add", rec.Path)
	assert.JSONEq(t, `{"emails":["a@x.dev","b@x.dev"],"origin":"manual"}`, string(rec.Body))
	assert.Len(t, res.Data, 2)

	c, rec = mockServer(t, 200, `{"data":[{"object":"suppression","id":"s1","deleted":true}]}`)
	rm, err := c.Suppressions.Batch.Remove(&BatchRemoveSuppressionsRequest{Ids: []string{"s1"}})
	require.NoError(t, err)
	assert.Equal(t, "/suppressions/batch/remove", rec.Path)
	assert.JSONEq(t, `{"ids":["s1"]}`, string(rec.Body))
	require.Len(t, rm.Data, 1)
	assert.True(t, rm.Data[0].Deleted)

	_, err = c.Suppressions.Batch.Remove(&BatchRemoveSuppressionsRequest{Emails: []string{"a@x.dev"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"emails":["a@x.dev"]}`, string(rec.Body))
}

func TestDomains(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"domain","id":"d1","name":"x.dev","status":"pending","created_at":"2026-01-01T00:00:00Z",
		"region":"us-east-1","open_tracking":false,"click_tracking":true,"tracking_subdomain":null,
		"capabilities":{"sending":"enabled","receiving":"disabled"},
		"records":[{"record":"DKIM","name":"a._domainkey","type":"CNAME","ttl":"Auto","status":"pending","value":"a.dkim.amazonses.com"},
		           {"record":"SPF","name":"send","type":"MX","ttl":"Auto","status":"pending","value":"feedback-smtp.us-east-1.amazonses.com","priority":10}]}`)
	d, err := c.Domains.Create(&CreateDomainRequest{
		Name: "x.dev", Region: "us-east-1", CustomReturnPath: "mail",
		OpenTracking: Ptr(false), ClickTracking: Ptr(true), TrackingSubdomain: "links",
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/domains", rec.Path)
	assert.JSONEq(t, `{"name":"x.dev","region":"us-east-1","custom_return_path":"mail","open_tracking":false,"click_tracking":true,"tracking_subdomain":"links"}`, string(rec.Body))
	assert.Equal(t, "d1", d.Id)
	assert.True(t, d.ClickTracking)
	assert.Equal(t, "", d.TrackingSubdomain)
	assert.Equal(t, "enabled", d.Capabilities.Sending)
	require.Len(t, d.Records, 2)
	assert.Equal(t, 10, d.Records[1].Priority)

	_, err = c.Domains.Get("d1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/domains/d1", rec.Path)

	_, err = c.Domains.Verify("d1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/domains/d1/verify", rec.Path)

	upd := &UpdateDomainRequest{OpenTracking: Ptr(true)}
	upd.ClearTrackingSubdomain()
	_, err = c.Domains.Update("d1", upd)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.JSONEq(t, `{"open_tracking":true,"tracking_subdomain":null}`, string(rec.Body))

	_, err = c.Domains.Update("d1", &UpdateDomainRequest{ClickTracking: Ptr(false), TrackingSubdomain: "go"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"click_tracking":false,"tracking_subdomain":"go"}`, string(rec.Body))

	_, err = c.Domains.Remove("d1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/domains/d1", rec.Path)

	c, rec = mockServer(t, 200, `{"object":"list","data":[{"id":"d1","name":"x.dev"}],"has_more":false}`)
	list, err := c.Domains.List(&ListOptions{Before: "cur"})
	require.NoError(t, err)
	assert.Equal(t, "before=cur", rec.RawQuery)
	assert.Equal(t, "x.dev", list.Data[0].Name)
}

func TestWebhooks(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"webhook","id":"w1","signing_secret":"whsec_abc"}`)
	created, err := c.Webhooks.Create(&CreateWebhookRequest{
		Endpoint: "https://x.dev/hook", Events: []string{"email.delivered", "email.bounced"}, SigningSecret: "whsec_abc",
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/webhooks", rec.Path)
	assert.JSONEq(t, `{"endpoint":"https://x.dev/hook","events":["email.delivered","email.bounced"],"signing_secret":"whsec_abc"}`, string(rec.Body))
	assert.Equal(t, "whsec_abc", created.SigningSecret)

	c, rec = mockServer(t, 200, `{"object":"webhook","id":"w1","endpoint":"https://x.dev/hook","created_at":"2026-01-01T00:00:00Z","status":"enabled","events":["email.sent"],"signing_secret":"whsec_abc","previous_secret_expires_at":null}`)
	got, err := c.Webhooks.Get("w1")
	require.NoError(t, err)
	assert.Equal(t, "/webhooks/w1", rec.Path)
	assert.Equal(t, "whsec_abc", got.SigningSecret)
	assert.Equal(t, []string{"email.sent"}, got.Events)
	assert.Nil(t, got.PreviousSecretExpiresAt)

	c, rec = mockServer(t, 200, `{"object":"webhook","id":"w1","endpoint":"https://x.dev/hook","created_at":"2026-01-01T00:00:00Z","status":"enabled","events":[],"signing_secret":"whsec_new","previous_secret_expires_at":"2026-01-02T00:00:00Z"}`)
	got, err = c.Webhooks.Get("w1")
	require.NoError(t, err)
	assert.Equal(t, "2026-01-02T00:00:00Z", *got.PreviousSecretExpiresAt)

	_, err = c.Webhooks.Update("w1", &UpdateWebhookRequest{Status: "disabled", Events: []string{"email.opened"}})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.JSONEq(t, `{"status":"disabled","events":["email.opened"]}`, string(rec.Body))

	_, err = c.Webhooks.Remove("w1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/webhooks/w1", rec.Path)

	c, rec = mockServer(t, 200, `{"object":"list","data":[{"id":"w1","events":null}],"has_more":false}`)
	list, err := c.Webhooks.List(nil)
	require.NoError(t, err)
	assert.Equal(t, "/webhooks", rec.Path)
	assert.Nil(t, list.Data[0].Events)
}

func TestWebhooksRotate(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"webhook","id":"w1","signing_secret":"whsec_new","previous_secret_expires_at":"2026-01-02T00:00:00Z"}`)
	res, err := c.Webhooks.Rotate("w1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/webhooks/w1/rotate", rec.Path)
	assert.JSONEq(t, `{}`, string(rec.Body))
	assert.Equal(t, "whsec_new", res.SigningSecret)
	assert.Equal(t, "2026-01-02T00:00:00Z", *res.PreviousSecretExpiresAt)

	_, err = c.Webhooks.Rotate("w1", &RotateWebhookSecretRequest{OverlapHours: Ptr(48)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"overlap_hours":48}`, string(rec.Body))

	// Zero is a real value (drop the old secret at once), not an omission.
	c, rec = mockServer(t, 200, `{"object":"webhook","id":"w1","signing_secret":"whsec_mine","previous_secret_expires_at":null}`)
	res, err = c.Webhooks.Rotate("w1", &RotateWebhookSecretRequest{SigningSecret: "whsec_mine", OverlapHours: Ptr(0)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"signing_secret":"whsec_mine","overlap_hours":0}`, string(rec.Body))
	assert.Nil(t, res.PreviousSecretExpiresAt)
}

func TestApiKeys(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"k1","token":"ms_secret"}`)
	created, err := c.ApiKeys.Create(&CreateApiKeyRequest{Name: "ci", Permission: "sending_access", DomainId: "d1"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/api-keys", rec.Path)
	assert.JSONEq(t, `{"name":"ci","permission":"sending_access","domain_id":"d1"}`, string(rec.Body))
	assert.Equal(t, "ms_secret", created.Token)

	c, rec = mockServer(t, 200, `{"object":"list","data":[{"id":"k1","name":"ci","created_at":"2026-01-01T00:00:00Z","last_used_at":null}],"has_more":false}`)
	list, err := c.ApiKeys.List(&ListOptions{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, "/api-keys", rec.Path)
	assert.Equal(t, "limit=2", rec.RawQuery)
	assert.Nil(t, list.Data[0].LastUsedAt)

	c, rec = mockServer(t, 200, `{"object":"api_key","id":"k1","deleted":true}`)
	rm, err := c.ApiKeys.Remove("k1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/api-keys/k1", rec.Path)
	assert.Equal(t, "k1", rm.Id)
	assert.True(t, rm.Deleted)
}

func TestTemplates(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"template","id":"tp1"}`)
	_, err := c.Templates.Create(&CreateTemplateRequest{Name: "Welcome", Html: "<p>{{{NAME}}}</p>", Subject: "Hi", Text: "t", Alias: "welcome"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/templates", rec.Path)
	assert.JSONEq(t, `{"name":"Welcome","html":"<p>{{{NAME}}}</p>","subject":"Hi","text":"t","alias":"welcome"}`, string(rec.Body))

	upd := &UpdateTemplateRequest{Name: "Welcome v2"}
	upd.ClearAlias()
	upd.ClearSubject()
	_, err = c.Templates.Update("welcome", upd)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/templates/welcome", rec.Path)
	assert.JSONEq(t, `{"name":"Welcome v2","alias":null,"subject":null}`, string(rec.Body))

	_, err = c.Templates.Publish("tp1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/templates/tp1/publish", rec.Path)

	_, err = c.Templates.Duplicate("tp1")
	require.NoError(t, err)
	assert.Equal(t, "/templates/tp1/duplicate", rec.Path)

	_, err = c.Templates.Remove("tp1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/templates/tp1", rec.Path)

	c, rec = mockServer(t, 200, `{"object":"template","id":"tp1","name":"Welcome","alias":null,"status":"published",
		"published_at":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z",
		"current_version_id":"v1","from":null,"subject":"Hi","reply_to":null,"html":"<p>h</p>","text":null,"variables":[],"has_unpublished_versions":false}`)
	got, err := c.Templates.Get("welcome")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/templates/welcome", rec.Path)
	assert.Equal(t, "published", got.Status)
	assert.Equal(t, "", got.Alias)
	assert.Equal(t, "<p>h</p>", got.Html)

	c, rec = mockServer(t, 200, `{"object":"list","data":[{"id":"tp1","name":"Welcome","alias":"welcome"}],"has_more":true}`)
	list, err := c.Templates.List(&ListOptions{After: "cur"})
	require.NoError(t, err)
	assert.Equal(t, "after=cur", rec.RawQuery)
	assert.True(t, list.HasMore)
	assert.Equal(t, "welcome", list.Data[0].Alias)
}

func TestContactProperties(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"contact_property","id":"p1","key":"plan","type":"string","fallback_value":"free","created_at":"2026-01-01T00:00:00Z"}`)
	created, err := c.ContactProperties.Create(&CreateContactPropertyRequest{Key: "plan", Type: "string", FallbackValue: "free"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/contact-properties", rec.Path)
	assert.JSONEq(t, `{"key":"plan","type":"string","fallback_value":"free"}`, string(rec.Body))
	assert.Equal(t, "free", created.FallbackValue)

	_, err = c.ContactProperties.Create(&CreateContactPropertyRequest{Key: "seats", Type: "number"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"seats","type":"number"}`, string(rec.Body))

	_, err = c.ContactProperties.Get("p1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/contact-properties/p1", rec.Path)

	_, err = c.ContactProperties.Update("p1", &UpdateContactPropertyRequest{FallbackValue: 5})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.JSONEq(t, `{"fallback_value":5}`, string(rec.Body))

	// nil clears the fallback: it must reach the wire as null.
	_, err = c.ContactProperties.Update("p1", &UpdateContactPropertyRequest{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"fallback_value":null}`, string(rec.Body))

	_, err = c.ContactProperties.Remove("p1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/contact-properties/p1", rec.Path)

	c, rec = mockServer(t, 200, `{"object":"list","data":[{"id":"p1","key":"seats","type":"number","fallback_value":null}],"has_more":false}`)
	list, err := c.ContactProperties.List(nil)
	require.NoError(t, err)
	assert.Equal(t, "/contact-properties", rec.Path)
	assert.Nil(t, list.Data[0].FallbackValue)
}

func TestUsageGet(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"usage","cloud":true,"plan":"pro","limits":{"emails_per_day":50000,"domains":10},
		"today":{"emails_sent":1234,"resets_at":"2026-09-05T00:00:00Z"},"team":{"id":"tm1","name":"Acme"},"app_url":"https://app.x.dev"}`)
	u, err := c.Usage.Get()
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/usage", rec.Path)
	assert.True(t, u.Cloud)
	assert.Equal(t, "pro", *u.Plan)
	assert.Equal(t, int64(50000), *u.Limits.EmailsPerDay)
	assert.Equal(t, 10, *u.Limits.Domains)
	assert.Equal(t, int64(1234), u.Today.EmailsSent)
	assert.Equal(t, "Acme", u.Team.Name)
	assert.Equal(t, "https://app.x.dev", *u.AppUrl)

	c, _ = mockServer(t, 200, `{"object":"usage","cloud":false,"plan":null,"limits":{"emails_per_day":null,"domains":null},
		"today":{"emails_sent":0,"resets_at":"2026-09-05T00:00:00Z"},"team":{"id":"tm1","name":"Self"},"app_url":null}`)
	u, err = c.Usage.Get()
	require.NoError(t, err)
	assert.Nil(t, u.Plan)
	assert.Nil(t, u.Limits.EmailsPerDay)
	assert.Nil(t, u.AppUrl)
}

// The Clear… helpers never leak into the wire as a field of their own.
func TestClearedRequestsHaveNoStrayKeys(t *testing.T) {
	r := &UpdateTemplateRequest{Html: "<p/>"}
	r.ClearText()
	b, err := json.Marshal(r)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	assert.Equal(t, map[string]any{"html": "<p/>", "text": nil}, m)
}

func TestSegmentsManualFilterAndClear(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"segment","id":"s1","name":"Manual","filter":null,"created_at":"2026-01-01T00:00:00Z"}`)
	seg, err := c.Segments.Create(&CreateSegmentRequest{Name: "Manual"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/segments", rec.Path)
	assert.JSONEq(t, `{"name":"Manual"}`, string(rec.Body))
	assert.Equal(t, SegmentFilter{}, seg.Filter)

	// The value key is required on the wire, presence ops included.
	_, err = c.Segments.Create(&CreateSegmentRequest{Name: "Pro", Filter: SegmentFilter{
		Match:      "all",
		Conditions: []SegmentCondition{{Field: "property:plan", Op: "equals", Value: "pro"}, {Field: "first_name", Op: "is_set"}},
	}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Pro","filter":{"match":"all","conditions":[
		{"field":"property:plan","op":"equals","value":"pro"},
		{"field":"first_name","op":"is_set","value":""}]}}`, string(rec.Body))

	upd := &UpdateSegmentRequest{Name: "Renamed"}
	upd.ClearFilter()
	_, err = c.Segments.Update("s1", upd)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/segments/s1", rec.Path)
	assert.JSONEq(t, `{"name":"Renamed","filter":null}`, string(rec.Body))

	_, err = c.Segments.Update("s1", &UpdateSegmentRequest{Filter: &SegmentFilter{Match: "any", Conditions: []SegmentCondition{{Field: "email", Op: "ends_with", Value: "@x.dev"}}}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"filter":{"match":"any","conditions":[{"field":"email","op":"ends_with","value":"@x.dev"}]}}`, string(rec.Body))
}

// Fields the API declares only to answer 422 must still reach the wire.
func TestUnsupportedFieldsPassThrough(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"template","id":"tp1"}`)
	_, err := c.Templates.Create(&CreateTemplateRequest{
		Name: "Welcome", Html: "<p>{{{NAME}}}</p>", From: "Acme <a@x.dev>", ReplyTo: []string{"r@x.dev"},
		Variables: []*TemplateVariable{{Key: "NAME", Type: "string", FallbackValue: "there"}},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Welcome","html":"<p>{{{NAME}}}</p>","from":"Acme <a@x.dev>","reply_to":["r@x.dev"],
		"variables":[{"key":"NAME","type":"string","fallback_value":"there"}]}`, string(rec.Body))

	_, err = c.Templates.Update("tp1", &UpdateTemplateRequest{ReplyTo: "r@x.dev", Variables: []*TemplateVariable{{Key: "N", Type: "number"}}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"reply_to":"r@x.dev","variables":[{"key":"N","type":"number"}]}`, string(rec.Body))

	c, rec = mockServer(t, 200, anyOK)
	_, err = c.Domains.Update("d1", &UpdateDomainRequest{Tls: "enforced", OpenTracking: Ptr(true)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"tls":"enforced","open_tracking":true}`, string(rec.Body))
}

// Fixtures in the shapes of the inbox wire contract: Resend's fields plus
// MillionSend's extension keys.
const draftJSON = `{"object":"inbox_draft","id":"d1","type":"reply","to":["ada@x.dev"],"cc":[],"bcc":[],
	"subject":"Re: Hi","html":null,"text":"Thanks","thread_id":"t1","reply_to_email_id":"m1","email_id":null,
	"created_at":"2026-10-12T10:00:00.000Z","updated_at":"2026-10-12T10:00:00.000Z","status":"pending_approval",
	"approval_reasons":["send_policy","ramp"],"forward":false,"decided_by":null,"decided_at":null,
	"expires_at":"2026-10-19T10:00:00.000Z"}`

const inboxJSON = `{"object":"inbox","id":"i1","name":"Signup bot","email_address":"signup-bot-7f3k@acme.agents.example",
	"domain_id":"sd1","receiving_address":null,"from_name":null,"unread":2,"drafts":1,
	"last_received":"2026-10-12T10:00:00.000Z","created_at":"2026-10-12T09:00:00.000Z","kind":"agent","shared":true,
	"expires_at":"2026-10-13T09:00:00.000Z","quarantined":1,"pending_approval":1}`

func TestReceivingListLongPollQueryAndHeld(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","has_more":false,"data":[{"id":"r1","from":"a@x.dev",
		"screening":{"verdict":"clean","sender_trust":"external"},"extracted":{"kinds":["code"]},"read":false,
		"inbox_id":"i1","thread_id":"t1","spam_verdict":"pass"}],
		"held":[{"id":"r2","inbox_id":"i1","reason":"not_allowlisted","created_at":"2026-10-12T10:00:00.000Z"}]}`)
	res, err := c.Emails.Receiving.List(&ListReceivedEmailsOptions{
		Limit: 10, Before: "r0", Wait: 30, Since: "2026-10-12T00:00:00Z", InboxId: "i1", To: "bot@x.dev",
		From: "stripe.com", SubjectContains: "code", Unread: Ptr(false), IncludeQuarantined: true,
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/emails/receiving", rec.Path)
	assert.Equal(t, "before=r0&from=stripe.com&inbox_id=i1&include_quarantined=true&limit=10"+
		"&since=2026-10-12T00%3A00%3A00Z&subject_contains=code&to=bot%40x.dev&unread=false&wait=30", rec.RawQuery)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "external", res.Data[0].Screening.SenderTrust)
	assert.Equal(t, []string{"code"}, res.Data[0].Extracted.Kinds)
	assert.False(t, *res.Data[0].Read)
	assert.Equal(t, []HeldEmail{{Id: "r2", InboxId: "i1", Reason: "not_allowlisted", CreatedAt: "2026-10-12T10:00:00.000Z"}}, res.Held)

	_, err = c.Emails.Receiving.List(nil)
	require.NoError(t, err)
	assert.Empty(t, rec.RawQuery)
}

// A long-poll outlives the client's own timeout by the time it may wait.
func TestLongPollStretchesTheClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[],"held":[]}`)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("ms_test")
	c.BaseURL = srv.URL
	c.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}

	_, err := c.Emails.Receiving.List(nil)
	require.Error(t, err)

	_, err = c.Emails.Receiving.List(&ListReceivedEmailsOptions{Wait: 1})
	require.NoError(t, err)
	_, err = c.Inboxes.Codes.Latest("i1", &LatestCodeOptions{Wait: 1})
	require.NoError(t, err)
	assert.Equal(t, 50*time.Millisecond, c.HTTPClient.Timeout)
}

func TestReceivingGetDecodesExtensionsAndUnknownValues(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"email","id":"r1","from":"noreply@github.com","to":["bot@acme.dev"],
		"subject":"Your code","html":"<p>482913</p>","html_format":"cid","text":"Your code is 482913",
		"headers":{"from":"GitHub <noreply@github.com>"},"authentication":{"spf":"pass","dkim":"pass","dmarc":"gray"},
		"raw":null,"attachments":[{"id":"a1","filename":null,"content_type":"image/png","content_id":"logo",
		"content_disposition":"inline","size":120}],"inbox_id":"i1","thread_id":"t1",
		"inboxes":[{"inbox_id":"i1","thread_id":"t1","verdict":"clean"}],
		"screening":{"verdict":"clean","sender_trust":"external","quarantine_reason":null,"reasons":["targets_agent"],
		"injection_score":3,"phishing_score":null,"hidden_text_removed":true},
		"spam_verdict":"gray","spam_score":31,"virus_verdict":"processing_failed","virus_signature":null,
		"extracted_text":"Your code is 482913",
		"extracted":{"kinds":["code"],"code":"482913","link":null,"link_domain_matches_sender":null,"ambiguous":false},
		"brand_new":{"verdict":"cosmic"}}`)
	got, err := c.Emails.Receiving.GetWithOptions(context.Background(), "r1", &GetReceivedEmailParams{HtmlFormat: Ptr("cid")})
	require.NoError(t, err)
	assert.Equal(t, "/emails/receiving/r1", rec.Path)
	assert.Equal(t, "html_format=cid", rec.RawQuery)
	assert.Nil(t, got.Raw)
	assert.Equal(t, "gray", got.Authentication.Dmarc)
	assert.Equal(t, "GitHub <noreply@github.com>", got.Headers["from"])
	assert.Equal(t, EmailAttachment{Id: "a1", ContentType: "image/png", ContentId: "logo", ContentDisposition: "inline", Size: 120}, got.Attachments[0])
	assert.Equal(t, []ReceivedEmailInbox{{InboxId: "i1", ThreadId: "t1", Verdict: "clean"}}, got.Inboxes)
	assert.Equal(t, 3, *got.Screening.InjectionScore)
	assert.Nil(t, got.Screening.PhishingScore)
	assert.True(t, got.Screening.HiddenTextRemoved)
	assert.Equal(t, "processing_failed", got.VirusVerdict)
	assert.Equal(t, 31, *got.SpamScore)
	assert.Equal(t, "482913", got.Extracted.Code)
	assert.Nil(t, got.Extracted.LinkDomainMatchesSender)

	_, err = c.Emails.Receiving.Get("r1")
	require.NoError(t, err)
	assert.Empty(t, rec.RawQuery)
}

func TestReceivingReplySentOrDrafted(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"email","id":"e9","thread_id":"t1","inbox_id":"i1"}`)
	res, err := c.Emails.Receiving.Reply("r1", &ReplyReceivedEmailRequest{
		Text: "Thanks", Html: "<p>Thanks</p>", Subject: "Re: Hi", Cc: []string{"c@x.dev"}, Bcc: []string{"b@x.dev"},
		ReplyAll: true, RequiresApproval: true, From: "bot@acme.dev", InboxId: "i1",
	}, WithIdempotencyKey("rk-1"))
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/emails/receiving/r1/reply", rec.Path)
	assert.Equal(t, "rk-1", rec.Header.Get("Idempotency-Key"))
	assert.JSONEq(t, `{"text":"Thanks","html":"<p>Thanks</p>","subject":"Re: Hi","cc":["c@x.dev"],"bcc":["b@x.dev"],
		"reply_all":true,"requires_approval":true,"from":"bot@acme.dev","inbox_id":"i1"}`, string(rec.Body))
	require.NotNil(t, res.Sent)
	assert.Nil(t, res.Draft)
	assert.Equal(t, ReceivedEmailSent{Object: "email", Id: "e9", ThreadId: "t1", InboxId: "i1"}, *res.Sent)

	c, rec = mockServer(t, 202, draftJSON)
	res, err = c.Emails.Receiving.Reply("r1", &ReplyReceivedEmailRequest{Text: "Thanks"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"text":"Thanks"}`, string(rec.Body))
	assert.Empty(t, rec.Header.Get("Idempotency-Key"))
	assert.Nil(t, res.Sent)
	require.NotNil(t, res.Draft)
	assert.Equal(t, "pending_approval", res.Draft.Status)
	assert.Equal(t, []string{"send_policy", "ramp"}, res.Draft.ApprovalReasons)
}

func TestReceivingForwardRemoveAndAttachments(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"email","id":"e9","thread_id":null,"inbox_id":null}`)
	res, err := c.Emails.Receiving.Forward("r1", &ForwardReceivedEmailRequest{
		To: []string{"ops@acme.dev"}, Text: "FYI", Mode: "attachment", From: "me@acme.dev",
	}, WithIdempotencyKey("fk-1"))
	require.NoError(t, err)
	assert.Equal(t, "/emails/receiving/r1/forward", rec.Path)
	assert.Equal(t, "fk-1", rec.Header.Get("Idempotency-Key"))
	assert.JSONEq(t, `{"to":["ops@acme.dev"],"text":"FYI","mode":"attachment","from":"me@acme.dev"}`, string(rec.Body))
	assert.Equal(t, "", res.Sent.ThreadId)

	c, rec = mockServer(t, 200, `{"object":"email","id":"r1","deleted":true}`)
	rm, err := c.Emails.Receiving.Remove("r1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/emails/receiving/r1", rec.Path)
	assert.True(t, rm.Deleted)

	c, rec = mockServer(t, 200, `{"object":"list","has_more":false,"data":[{"id":"a1","filename":"invoice.pdf","size":48213,
		"content_type":"application/pdf","content_disposition":"attachment","download_url":"https://api.test/d/a1",
		"expires_at":"2026-10-12T11:00:00.000Z"}]}`)
	list, err := c.Emails.Receiving.Attachments.List("r1", &ListOptions{Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, "/emails/receiving/r1/attachments", rec.Path)
	assert.Equal(t, "limit=5", rec.RawQuery)
	assert.Equal(t, "https://api.test/d/a1", list.Data[0].DownloadUrl)

	_, err = c.Emails.Receiving.Attachments.Get("r1", "a1")
	require.NoError(t, err)
	assert.Equal(t, "/emails/receiving/r1/attachments/a1", rec.Path)
}

func TestInboxesCreateListGetUpdateRemove(t *testing.T) {
	c, rec := mockServer(t, 201, inboxJSON)
	inbox, err := c.Inboxes.Create(nil)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/inboxes", rec.Path)
	assert.JSONEq(t, `{}`, string(rec.Body))
	assert.Equal(t, Inbox{
		Object: "inbox", Id: "i1", Name: "Signup bot", EmailAddress: "signup-bot-7f3k@acme.agents.example", DomainId: "sd1",
		Unread: 2, Drafts: 1, LastReceived: "2026-10-12T10:00:00.000Z", CreatedAt: "2026-10-12T09:00:00.000Z",
		Kind: "agent", Shared: true, ExpiresAt: "2026-10-13T09:00:00.000Z", Quarantined: 1, PendingApproval: 1,
	}, *inbox)

	_, err = c.Inboxes.Create(&CreateInboxRequest{
		EmailAddress: "bot@acme.dev", Name: "Bot", FromName: "Acme Bot", Forwarding: true, Kind: "human", ExpiresIn: "7d",
		Safety: &InboxSafetyUpdate{AcceptFrom: "allowlist", Allowlist: Ptr([]string{"*@stripe.com"}), Screening: Ptr(false)},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"email_address":"bot@acme.dev","name":"Bot","from_name":"Acme Bot","forwarding":true,"kind":"human",
		"expires_in":"7d","safety":{"accept_from":"allowlist","allowlist":["*@stripe.com"],"screening":false}}`, string(rec.Body))

	_, err = c.Inboxes.List(&ListInboxesOptions{Kind: "agent", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes", rec.Path)
	assert.Equal(t, "kind=agent&limit=2", rec.RawQuery)

	_, err = c.Inboxes.Get("bot+sales@acme.dev")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/inboxes/bot+sales@acme.dev", rec.Path)

	upd := &UpdateInboxRequest{Name: "Renamed", FromName: Ptr("")}
	upd.ClearExpiresIn()
	_, err = c.Inboxes.Update("i1", upd)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/inboxes/i1", rec.Path)
	assert.JSONEq(t, `{"name":"Renamed","from_name":"","expires_in":null}`, string(rec.Body))

	c, rec = mockServer(t, 200, `{"object":"inbox","id":"i1","deleted":true}`)
	rm, err := c.Inboxes.Remove("i1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/inboxes/i1", rec.Path)
	assert.True(t, rm.Deleted)
}

func TestInboxSafetyCodesAndQuarantine(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"inbox_safety","inbox_id":"i1","accept_from":"verified","allowlist":[],
		"owner_addresses":["ops@acme.dev"],"screening":true,"extract_codes":true,"send_policy":"reply_only",
		"require_approval":false,"private_codes":true,"send_cap":null,"forward_to":[]}`)
	safety, err := c.Inboxes.Safety.Get("i1")
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/safety", rec.Path)
	assert.Nil(t, safety.SendCap)
	assert.Equal(t, []string{"ops@acme.dev"}, safety.OwnerAddresses)
	assert.Equal(t, "reply_only", safety.SendPolicy)

	upd := &InboxSafetyUpdate{Allowlist: Ptr([]string{}), RequireApproval: Ptr(true), SendPolicy: "allowlist"}
	upd.ClearSendCap()
	_, err = c.Inboxes.Safety.Update("i1", upd)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.JSONEq(t, `{"allowlist":[],"require_approval":true,"send_policy":"allowlist","send_cap":null}`, string(rec.Body))

	c, rec = mockServer(t, 200, `{"object":"verification_code","status":"found","kind":"otp","code":"482913","url":null,
		"link_domain_matches_sender":null,"email_id":"r1","from":"noreply@github.com",
		"received_at":"2026-10-12T10:00:00.000Z","sender_trust":"external"}`)
	code, err := c.Inboxes.Codes.Latest("i1", &LatestCodeOptions{From: "github.com", Wait: 20, MaxAge: 600})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/codes/latest", rec.Path)
	assert.Equal(t, "from=github.com&max_age=600&wait=20", rec.RawQuery)
	assert.Equal(t, "482913", code.Code)
	assert.Nil(t, code.LinkDomainMatchesSender)
	_, err = c.Inboxes.Codes.Latest("i1", nil)
	require.NoError(t, err)
	assert.Empty(t, rec.RawQuery)

	c, rec = mockServer(t, 200, `{"object":"list","has_more":false,"data":[{"id":"r1","thread_id":"t1","from":"x@y.dev",
		"subject":"Hi","received_at":"2026-10-12T10:00:00.000Z","quarantine_reason":"injection",
		"sender_trust":"external","injection_score":97,"phishing_score":null,"virus_signature":null}]}`)
	held, err := c.Inboxes.Quarantine.List("i1", &ListOptions{Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/quarantine", rec.Path)
	assert.Equal(t, "limit=5", rec.RawQuery)
	assert.Equal(t, 97, *held.Data[0].InjectionScore)

	c, rec = mockServer(t, 200, `{"object":"inbox_release","inbox_id":"i1","email_id":"r1",
		"released_at":"2026-10-12T10:05:00.000Z","trusted_sender":"x@y.dev"}`)
	rel, err := c.Inboxes.Quarantine.Release("i1", "r1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/inboxes/i1/quarantine/r1/release", rec.Path)
	assert.JSONEq(t, `{}`, string(rec.Body))
	assert.Equal(t, "x@y.dev", rel.TrustedSender)
	_, err = c.Inboxes.Quarantine.Release("i1", "r1", &ReleaseQuarantinedRequest{TrustSender: true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"trust_sender":true}`, string(rec.Body))
}

func TestInboxLabels(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","data":[{"id":"l1","name":"Billing","color":"#FFAA00","created_at":"2026-10-12T10:00:00.000Z"}]}`)
	labels, err := c.Inboxes.Labels.List("i1")
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/labels", rec.Path)
	assert.Equal(t, "#FFAA00", labels.Data[0].Color)

	c, rec = mockServer(t, 201, `{"object":"inbox_label","id":"l1","name":"Billing","color":"#FFAA00","created_at":"2026-10-12T10:00:00.000Z"}`)
	_, err = c.Inboxes.Labels.Create("i1", &CreateInboxLabelRequest{Name: "Billing"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.JSONEq(t, `{"name":"Billing"}`, string(rec.Body))

	_, err = c.Inboxes.Labels.Update("i1", "l1", &UpdateInboxLabelRequest{Color: "#00AAFF"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/inboxes/i1/labels/l1", rec.Path)
	assert.JSONEq(t, `{"color":"#00AAFF"}`, string(rec.Body))

	_, err = c.Inboxes.Labels.Remove("i1", "l1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/inboxes/i1/labels/l1", rec.Path)
}

func TestInboxThreads(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","has_more":true,"data":[{"id":"t1","subject":"Invoice 42",
		"from":"Stripe <billing@stripe.com>","to":["bot@acme.dev"],"cc":[],"bcc":[],
		"labels":[{"id":"l1","name":"Billing","color":"#FFAA00"}],"message_count":2,"has_attachment":true,
		"has_draft":false,"read":true,"received_at":"2026-10-12T10:00:00.000Z","folder":"quarantine"}]}`)
	list, err := c.Inboxes.Threads.List("i1", &ListInboxThreadsOptions{
		Folders: []string{"inbox", "archive"}, Labels: []string{"l1", "l2"}, Read: Ptr(false), IncludeQuarantined: true,
		Limit: 20, After: "t0",
	})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/threads", rec.Path)
	assert.Equal(t, "after=t0&folders=inbox%2Carchive&include_quarantined=true&labels=l1%2Cl2&limit=20&read=false", rec.RawQuery)
	assert.Equal(t, "quarantine", list.Data[0].Folder) // an enum value this SDK never saw still decodes
	assert.Equal(t, "Billing", list.Data[0].Labels[0].Name)

	c, rec = mockServer(t, 200, `{"object":"list","has_more":false,"data":[{"id":"t1","matched_email_id":"m2",
		"highlights":{"subject":["**Invoice** 42"]}}]}`)
	found, err := c.Inboxes.Threads.Search("i1", &SearchInboxThreadsOptions{
		Query: `"invoice 42" -spam`, From: []string{"stripe.com", "ada"}, To: []string{"bot@acme.dev"},
		HasAttachment: Ptr(true), StartDate: "2026-10-01", EndDate: "2026-10-12", Folders: []string{"sent"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/threads/search", rec.Path)
	assert.Equal(t, "end_date=2026-10-12&folders=sent&from=stripe.com%2Cada&has_attachment=true"+
		"&query=%22invoice+42%22+-spam&start_date=2026-10-01&to=bot%40acme.dev", rec.RawQuery)
	assert.Equal(t, "m2", found.Data[0].MatchedEmailId)
	assert.Equal(t, []string{"**Invoice** 42"}, found.Data[0].Highlights["subject"])

	c, rec = mockServer(t, 200, `{"object":"inbox_thread","id":"t1","subject":null,"folder":"archive","labels":[],"read":true,"quarantined":0}`)
	th, err := c.Inboxes.Threads.Get("i1", "t1")
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/threads/t1", rec.Path)
	assert.Equal(t, "archive", th.Folder)

	_, err = c.Inboxes.Threads.Update("i1", "t1", &UpdateInboxThreadRequest{Read: Ptr(false), Folder: "archive", LabelId: "l1"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.JSONEq(t, `{"read":false,"folder":"archive","label_id":"l1"}`, string(rec.Body))

	c, rec = mockServer(t, 200, `{"object":"inbox_thread","id":"t1","deleted":false}`)
	rm, err := c.Inboxes.Threads.Remove("i1", "t1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.False(t, rm.Deleted)
}

func TestInboxThreadEmails(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","has_more":false,"data":[{"id":"m1","direction":"inbound",
		"from":"Ada <ada@x.dev>","to":["bot@acme.dev"],"cc":[],"bcc":[],"reply_to":[],"subject":"Hi",
		"message_id":"<a@x.dev>","html":null,"text":"Hello","attachments":[{"id":"a1","filename":null,"size":null}],
		"read":false,"received_at":"2026-10-12T10:00:00.000Z","extracted_text":"Hello","spam_verdict":"pass",
		"virus_verdict":"pass","screening":{"verdict":"suspicious","sender_trust":"unauthenticated","quarantine_reason":null,
		"reasons":[],"injection_score":null,"phishing_score":null,"hidden_text_removed":false}}]}`)
	list, err := c.Inboxes.Threads.Emails.List("i1", "t1", &ListOptions{After: "m0"})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/threads/t1/emails", rec.Path)
	assert.Equal(t, "after=m0", rec.RawQuery)
	assert.Equal(t, "suspicious", list.Data[0].Screening.Verdict)
	assert.Equal(t, EmailAttachment{Id: "a1"}, list.Data[0].Attachments[0])

	_, err = c.Inboxes.Threads.Emails.Get("i1", "t1", "m1")
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/threads/t1/emails/m1", rec.Path)

	c, rec = mockServer(t, 200, `{"id":"m2","direction":"outbound","from":"Signup bot <bot@acme.dev>","to":["ada@x.dev"],
		"cc":[],"bcc":[],"reply_to":[],"subject":"Re: Hi","message_id":null,"html":null,"text":"Thanks","attachments":[],
		"read":true,"received_at":"2026-10-12T10:01:00.000Z","email_id":"m2","thread_id":"t1","inbox_id":"i1","status":"queued"}`)
	res, err := c.Inboxes.Threads.Emails.Reply("i1", "t1", "m1", &ReplyInThreadRequest{Text: "Thanks", ReplyAll: true}, WithIdempotencyKey("tr-1"))
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/threads/t1/emails/m1/reply", rec.Path)
	assert.Equal(t, "tr-1", rec.Header.Get("Idempotency-Key"))
	assert.JSONEq(t, `{"text":"Thanks","reply_all":true}`, string(rec.Body))
	assert.Equal(t, "m2", res.Sent.EmailId)
	assert.Equal(t, "queued", res.Sent.Status)

	c, rec = mockServer(t, 202, draftJSON)
	res, err = c.Inboxes.Threads.Emails.Forward("i1", "t1", "m1", &ForwardInThreadRequest{To: []string{"ops@acme.dev"}, Subject: "Fwd: Hi", RequiresApproval: true})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/threads/t1/emails/m1/forward", rec.Path)
	assert.JSONEq(t, `{"to":["ops@acme.dev"],"subject":"Fwd: Hi","requires_approval":true}`, string(rec.Body))
	assert.Nil(t, res.Sent)
	assert.Equal(t, "d1", res.Draft.Id)
}

func TestInboxDrafts(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","has_more":false,"data":[{"id":"d1","type":"standalone","to":null,
		"cc":[],"bcc":[],"subject":null,"snippet":"Hi Ada","thread_id":null,"reply_to_email_id":null,
		"updated_at":"2026-10-12T10:00:00.000Z","status":"draft","approval_reasons":[]}]}`)
	list, err := c.Inboxes.Drafts.List("i1", &ListOptions{Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/drafts", rec.Path)
	assert.Equal(t, "limit=3", rec.RawQuery)
	assert.Nil(t, list.Data[0].To)
	assert.Equal(t, "Hi Ada", list.Data[0].Snippet)

	c, rec = mockServer(t, 201, draftJSON)
	_, err = c.Inboxes.Drafts.Create("i1", &CreateInboxDraftRequest{
		To: []string{"ada@x.dev"}, Subject: "Re: Hi", Text: "Thanks", ThreadId: "t1", ReplyToEmailId: "m1",
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.JSONEq(t, `{"to":["ada@x.dev"],"subject":"Re: Hi","text":"Thanks","thread_id":"t1","reply_to_email_id":"m1"}`, string(rec.Body))

	got, err := c.Inboxes.Drafts.Get("i1", "d1")
	require.NoError(t, err)
	assert.Equal(t, "/inboxes/i1/drafts/d1", rec.Path)
	assert.Equal(t, "2026-10-19T10:00:00.000Z", got.ExpiresAt)

	upd := &UpdateInboxDraftRequest{Html: "<p>Thanks</p>"}
	upd.ClearSubject()
	upd.ClearCc()
	_, err = c.Inboxes.Drafts.Update("i1", "d1", upd)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.JSONEq(t, `{"html":"<p>Thanks</p>","subject":null,"cc":null}`, string(rec.Body))

	_, err = c.Inboxes.Drafts.Remove("i1", "d1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)

	_, err = c.Inboxes.Drafts.Reject("i1", "d1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/inboxes/i1/drafts/d1/reject", rec.Path)
	assert.Empty(t, rec.Body)
}

func TestInboxDraftSendSentOrHeld(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"inbox_draft","id":"d1","thread_id":"t1","email_id":"e9"}`)
	res, err := c.Inboxes.Drafts.Send("i1", "d1", WithIdempotencyKey("ds-1"))
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/inboxes/i1/drafts/d1/send", rec.Path)
	assert.Equal(t, "ds-1", rec.Header.Get("Idempotency-Key"))
	assert.Empty(t, rec.Body)
	assert.Equal(t, InboxDraftSent{Object: "inbox_draft", Id: "d1", ThreadId: "t1", EmailId: "e9"}, *res.Sent)

	c, _ = mockServer(t, 202, draftJSON)
	res, err = c.Inboxes.Drafts.Send("i1", "d1")
	require.NoError(t, err)
	assert.Nil(t, res.Sent)
	assert.Equal(t, "pending_approval", res.Draft.Status)

	c, _ = mockServer(t, 403, `{"statusCode":403,"name":"approval_required","message":"A person must approve this draft"}`)
	_, err = c.Inboxes.Drafts.Send("i1", "d1")
	var mse *MillionSendError
	require.ErrorAs(t, err, &mse)
	assert.Equal(t, "approval_required", mse.Name)
}

func TestInboxFieldsOnExistingResources(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"domain","id":"d1","receiving_catch_all":true}`)
	d, err := c.Domains.Update("d1", &UpdateDomainRequest{ReceivingCatchAll: Ptr(true)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"receiving_catch_all":true}`, string(rec.Body))
	assert.True(t, d.ReceivingCatchAll)

	c, rec = mockServer(t, 200, `{"id":"k1","token":"ms_x"}`)
	_, err = c.ApiKeys.Create(&CreateApiKeyRequest{Name: "agent", Permission: "full_access", InboxId: "i1"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"agent","permission":"full_access","inbox_id":"i1"}`, string(rec.Body))

	c, _ = mockServer(t, 200, `{"object":"list","has_more":false,"data":[{"id":"k1","name":"agent","created_at":"2026-10-12T10:00:00.000Z",
		"last_used_at":null,"inbox_id":"i1"},{"id":"k2","name":"ci","created_at":"2026-10-12T10:00:00.000Z","last_used_at":null,"inbox_id":null}]}`)
	keys, err := c.ApiKeys.List(nil)
	require.NoError(t, err)
	assert.Equal(t, "i1", *keys.Data[0].InboxId)
	assert.Nil(t, keys.Data[1].InboxId)

	c, rec = mockServer(t, 200, `{"object":"webhook","id":"w1","signing_secret":"whsec_x"}`)
	_, err = c.Webhooks.Create(&CreateWebhookRequest{Endpoint: "https://x.dev/h", Events: []string{"email.received", "inbox.email.received"}, InboxIds: []string{"i1"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"endpoint":"https://x.dev/h","events":["email.received","inbox.email.received"],"inbox_ids":["i1"]}`, string(rec.Body))

	upd := &UpdateWebhookRequest{Status: "enabled"}
	upd.ClearInboxIds()
	_, err = c.Webhooks.Update("w1", upd)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"enabled","inbox_ids":null}`, string(rec.Body))
	_, err = c.Webhooks.Update("w1", &UpdateWebhookRequest{InboxIds: []string{"i1", "i2"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"inbox_ids":["i1","i2"]}`, string(rec.Body))

	c, _ = mockServer(t, 200, `{"object":"webhook","id":"w1","endpoint":"https://x.dev/h","events":["inbox.email.received"],
		"status":"enabled","created_at":"2026-10-12T10:00:00.000Z","inbox_ids":["i1"]}`)
	w, err := c.Webhooks.Get("w1")
	require.NoError(t, err)
	assert.Equal(t, []string{"i1"}, w.InboxIds)

	c, _ = mockServer(t, 200, `{"object":"usage","cloud":true,"plan":"starter","limits":{"emails_per_day":null,"domains":null,"inboxes":25},
		"today":{"emails_sent":0,"resets_at":"2026-10-13T00:00:00Z"},"team":{"id":"tm1","name":"Acme"},"app_url":null}`)
	u, err := c.Usage.Get()
	require.NoError(t, err)
	assert.Equal(t, 25, *u.Limits.Inboxes)
}
