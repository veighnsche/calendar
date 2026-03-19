package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"calendar-api/internal/config"
	"calendar-api/internal/events"
)

var (
	ErrCalendarNotFound  = errors.New("calendar not found")
	ErrEventNotFound     = errors.New("event not found")
	ErrCalDAVUnavailable = errors.New("caldav unavailable")
	ErrWriteConflict     = errors.New("write conflict")
)

type Client struct {
	baseURL    *url.URL
	username   string
	password   string
	defaultLoc *time.Location
	logger     *slog.Logger
	httpClient *http.Client
}

type HealthStatus struct {
	Reachable      bool   `json:"reachable"`
	UserCollection string `json:"userCollection"`
}

type Object struct {
	Calendar string
	ID       string
	ETag     string
	Data     []byte
	Event    events.Event
}

type ListOptions struct {
	Calendar string
	From     *time.Time
	To       *time.Time
	Limit    int
	Query    string
	Expand   bool
}

func NewClient(cfg config.Config, logger *slog.Logger) (*Client, error) {
	baseURL, err := url.Parse(cfg.CalDAVBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse CALDAV_BASE_URL: %w", err)
	}
	loc, err := cfg.DefaultLocation()
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL:    baseURL,
		username:   cfg.CalDAVUsername,
		password:   cfg.CalDAVPassword,
		defaultLoc: loc,
		logger:     logger,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *Client) Health(ctx context.Context) (HealthStatus, error) {
	body := `<d:propfind xmlns:d="DAV:"><d:prop><d:displayname/></d:prop></d:propfind>`
	req, err := c.newXMLRequest(ctx, "PROPFIND", c.userCollectionURL(), body)
	if err != nil {
		return HealthStatus{}, err
	}
	req.Header.Set("Depth", "0")

	resp, err := c.do(req)
	if err != nil {
		return HealthStatus{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		return HealthStatus{}, c.mapStatus(resp.StatusCode, ErrCalDAVUnavailable)
	}
	return HealthStatus{
		Reachable:      true,
		UserCollection: c.userCollectionURL(),
	}, nil
}

func (c *Client) ListCalendars(ctx context.Context) ([]events.Calendar, error) {
	body := `<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:displayname/><d:resourcetype/></d:prop></d:propfind>`
	req, err := c.newXMLRequest(ctx, "PROPFIND", c.userCollectionURL(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrCalDAVUnavailable
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, c.mapStatus(resp.StatusCode, ErrCalDAVUnavailable)
	}

	var result multiStatus
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: invalid XML response", ErrCalDAVUnavailable)
	}

	cals := make([]events.Calendar, 0)
	userHref := normalizedHref(c.userCollectionURL())
	for _, res := range result.Responses {
		href := normalizedHref(res.Href)
		if href == userHref {
			continue
		}
		prop, ok := res.okProp()
		if !ok || prop.ResourceType.Calendar == nil {
			continue
		}
		name := lastPathSegment(href)
		if name == "" {
			continue
		}
		displayName := strings.TrimSpace(prop.DisplayName)
		if displayName == "" {
			displayName = name
		}
		cals = append(cals, events.Calendar{
			Name:        name,
			DisplayName: displayName,
			Href:        href,
			Source:      events.SourceCalDAV,
		})
	}
	sort.Slice(cals, func(i, j int) bool { return cals[i].Name < cals[j].Name })
	return cals, nil
}

func (c *Client) ListEvents(ctx context.Context, opts ListOptions) ([]events.Event, error) {
	resp, err := c.calendarQuery(ctx, opts)
	if err != nil {
		return nil, err
	}
	items, err := c.decodeEvents(opts.Calendar, resp)
	if err != nil {
		return nil, err
	}
	items = filterEvents(items, opts.Query)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Start.Equal(items[j].Start) {
			return items[i].End.Before(items[j].End)
		}
		return items[i].Start.Before(items[j].Start)
	})
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}
	return items, nil
}

func (c *Client) UpcomingEvents(ctx context.Context, calendar string, limit int) ([]events.Event, error) {
	now := time.Now().In(c.defaultLoc)
	seen := make(map[string]struct{})
	result := make([]events.Event, 0, limit)
	horizon := 30 * 24 * time.Hour
	maxHorizon := 10 * 365 * 24 * time.Hour

	for len(result) < limit && horizon <= maxHorizon {
		from := now
		to := now.Add(horizon)
		items, err := c.ListEvents(ctx, ListOptions{
			Calendar: calendar,
			From:     &from,
			To:       &to,
			Limit:    0,
			Expand:   true,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.End.Before(now) {
				continue
			}
			key := item.Calendar + "\x00" + item.ID + "\x00" + item.Start.Format(time.RFC3339) + "\x00" + item.End.Format(time.RFC3339)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, item)
		}
		horizon *= 2
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Start.Equal(result[j].Start) {
			return result[i].End.Before(result[j].End)
		}
		return result[i].Start.Before(result[j].Start)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (c *Client) GetObject(ctx context.Context, calendarName, id string) (Object, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.objectURL(calendarName, id), nil)
	if err != nil {
		return Object{}, err
	}
	resp, err := c.do(req)
	if err != nil {
		return Object{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Object{}, ErrEventNotFound
	default:
		return Object{}, c.mapStatus(resp.StatusCode, ErrCalDAVUnavailable)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Object{}, fmt.Errorf("%w: read upstream response", ErrCalDAVUnavailable)
	}
	event, err := events.NormalizeSingleEvent(calendarName, cleanObjectID(id), data, resp.Header.Get("ETag"), c.defaultLoc)
	if err != nil {
		return Object{}, fmt.Errorf("%w: invalid calendar data", ErrCalDAVUnavailable)
	}
	return Object{
		Calendar: calendarName,
		ID:       cleanObjectID(id),
		ETag:     resp.Header.Get("ETag"),
		Data:     data,
		Event:    event,
	}, nil
}

func (c *Client) PutObject(ctx context.Context, calendarName, id string, data []byte, etag string, ifNoneMatch bool) (Object, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.objectURL(calendarName, id), bytes.NewReader(data))
	if err != nil {
		return Object{}, err
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	if ifNoneMatch {
		req.Header.Set("If-None-Match", "*")
	}
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}

	resp, err := c.do(req)
	if err != nil {
		return Object{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusNoContent, http.StatusOK:
	case http.StatusNotFound:
		return Object{}, ErrCalendarNotFound
	case http.StatusPreconditionFailed, http.StatusConflict:
		return Object{}, ErrWriteConflict
	default:
		return Object{}, c.mapStatus(resp.StatusCode, ErrCalDAVUnavailable)
	}
	return c.GetObject(ctx, calendarName, id)
}

func (c *Client) DeleteObject(ctx context.Context, calendarName, id, etag string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.objectURL(calendarName, id), nil)
	if err != nil {
		return err
	}
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrEventNotFound
	case http.StatusPreconditionFailed, http.StatusConflict:
		return ErrWriteConflict
	default:
		return c.mapStatus(resp.StatusCode, ErrCalDAVUnavailable)
	}
}

func (c *Client) calendarQuery(ctx context.Context, opts ListOptions) (multiStatus, error) {
	body := buildCalendarQueryBody(opts.From, opts.To, opts.Expand)
	req, err := c.newXMLRequest(ctx, "REPORT", c.calendarURL(opts.Calendar), body)
	if err != nil {
		return multiStatus{}, err
	}
	req.Header.Set("Depth", "1")

	resp, err := c.do(req)
	if err != nil {
		return multiStatus{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusMultiStatus:
	case http.StatusNotFound:
		return multiStatus{}, ErrCalendarNotFound
	default:
		return multiStatus{}, c.mapStatus(resp.StatusCode, ErrCalDAVUnavailable)
	}

	var result multiStatus
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return multiStatus{}, fmt.Errorf("%w: invalid XML response", ErrCalDAVUnavailable)
	}
	return result, nil
}

func (c *Client) decodeEvents(calendarName string, response multiStatus) ([]events.Event, error) {
	items := make([]events.Event, 0)
	for _, res := range response.Responses {
		prop, ok := res.okProp()
		if !ok || strings.TrimSpace(prop.CalendarData) == "" {
			continue
		}
		id := lastPathSegment(res.Href)
		id = cleanObjectID(id)
		normalized, err := events.NormalizeCalendarObject(calendarName, id, []byte(prop.CalendarData), prop.GetETag, c.defaultLoc)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid calendar data", ErrCalDAVUnavailable)
		}
		items = append(items, normalized...)
	}
	return items, nil
}

func (c *Client) newXMLRequest(ctx context.Context, method, rawURL, body string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("caldav request failed", "method", req.Method, "url", req.URL.String(), "error", err)
		return nil, ErrCalDAVUnavailable
	}
	if resp.StatusCode >= 500 {
		c.logger.Error("caldav upstream error", "method", req.Method, "url", req.URL.String(), "status", resp.StatusCode)
	}
	return resp, nil
}

func (c *Client) mapStatus(status int, fallback error) error {
	switch status {
	case http.StatusNotFound:
		return ErrEventNotFound
	case http.StatusPreconditionFailed, http.StatusConflict:
		return ErrWriteConflict
	default:
		return fallback
	}
}

func (c *Client) userCollectionURL() string {
	return c.collectionURL(c.username)
}

func (c *Client) calendarURL(calendarName string) string {
	return c.collectionURL(c.username, calendarName)
}

func (c *Client) objectURL(calendarName, id string) string {
	u := *c.baseURL
	u.Path = joinURLPath(c.baseURL.Path, c.username, calendarName, objectName(id))
	return u.String()
}

func (c *Client) collectionURL(parts ...string) string {
	u := *c.baseURL
	u.Path = joinURLPath(c.baseURL.Path, parts...)
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u.String()
}

func buildCalendarQueryBody(from, to *time.Time, expand bool) string {
	var body strings.Builder
	body.WriteString(`<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:getetag/><c:calendar-data>`)
	if expand && from != nil && to != nil {
		body.WriteString(`<c:expand start="`)
		body.WriteString(from.UTC().Format("20060102T150405Z"))
		body.WriteString(`" end="`)
		body.WriteString(to.UTC().Format("20060102T150405Z"))
		body.WriteString(`"/>`)
	}
	body.WriteString(`</c:calendar-data></d:prop>`)
	if from != nil && to != nil {
		body.WriteString(`<c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"><c:time-range start="`)
		body.WriteString(from.UTC().Format("20060102T150405Z"))
		body.WriteString(`" end="`)
		body.WriteString(to.UTC().Format("20060102T150405Z"))
		body.WriteString(`"/></c:comp-filter></c:comp-filter></c:filter>`)
	}
	body.WriteString(`</c:calendar-query>`)
	return body.String()
}

func filterEvents(items []events.Event, query string) []events.Event {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return items
	}
	filtered := make([]events.Event, 0, len(items))
	for _, item := range items {
		haystack := strings.ToLower(strings.Join([]string{item.ID, item.Title, item.Description, item.Location}, "\n"))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func joinURLPath(base string, parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	if trimmed := strings.Trim(strings.TrimSpace(base), "/"); trimmed != "" {
		segments = append(segments, strings.Split(trimmed, "/")...)
	}
	for _, part := range parts {
		if trimmed := strings.Trim(strings.TrimSpace(part), "/"); trimmed != "" {
			segments = append(segments, url.PathEscape(trimmed))
		}
	}
	if len(segments) == 0 {
		return "/"
	}
	return "/" + strings.Join(segments, "/")
}

func objectName(id string) string {
	id = cleanObjectID(id)
	if strings.HasSuffix(strings.ToLower(id), ".ics") {
		return id
	}
	return id + ".ics"
}

func cleanObjectID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.TrimSuffix(id, ".ics")
	id = strings.TrimSuffix(id, ".ICS")
	return id
}

func normalizedHref(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	decoded, err := url.PathUnescape(strings.TrimSuffix(u.Path, "/"))
	if err != nil {
		return strings.TrimSuffix(u.Path, "/")
	}
	return decoded
}

func lastPathSegment(raw string) string {
	u, err := url.Parse(raw)
	if err == nil {
		raw = u.Path
	}
	raw = strings.TrimSuffix(raw, "/")
	base := path.Base(raw)
	decoded, err := url.PathUnescape(base)
	if err != nil {
		return base
	}
	return decoded
}

type multiStatus struct {
	Responses []response `xml:"response"`
}

type response struct {
	Href     string     `xml:"href"`
	PropStat []propStat `xml:"propstat"`
}

type propStat struct {
	Status string `xml:"status"`
	Prop   prop   `xml:"prop"`
}

type prop struct {
	DisplayName  string       `xml:"displayname"`
	GetETag      string       `xml:"getetag"`
	CalendarData string       `xml:"calendar-data"`
	ResourceType resourceType `xml:"resourcetype"`
}

type resourceType struct {
	Collection *struct{} `xml:"collection"`
	Calendar   *struct{} `xml:"calendar"`
}

func (r response) okProp() (prop, bool) {
	for _, ps := range r.PropStat {
		if strings.Contains(ps.Status, " 200 ") {
			return ps.Prop, true
		}
	}
	return prop{}, false
}
