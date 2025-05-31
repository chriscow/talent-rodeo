package funcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	day  = 24 * time.Hour
	year = 365 * day
)

var printer = message.NewPrinter(language.English)

var TemplateFuncs = template.FuncMap{
	// Time functions
	"now":            time.Now,
	"timeSince":      time.Since,
	"timeUntil":      time.Until,
	"formatTime":     formatTime,
	"approxAge":      ApproxAge,
	"approxDuration": ApproxDuration,
	"humanDate":      humanDate,

	// String functions
	"uppercase": strings.ToUpper,
	"lowercase": strings.ToLower,
	"pluralize": pluralize,
	"slugify":   Slugify,
	"safeHTML":  safeHTML,
	"nullOrEmpty": func(s *string) bool {
		return s == nil || *s == ""
	},
	"hasValue": func(s *string) bool {
		return s != nil && *s != ""
	},

	// Slice functions
	"join": strings.Join,

	// Number functions
	"incr":        incr,
	"decr":        decr,
	"formatInt":   formatInt,
	"formatFloat": formatFloat,
	"mul":         mul,
	"percentage":  percentage,
	"add": func(a, b int) int {
		return a + b
	},
	"rrfPercentage": rrfPercentage,
	"rrfStars":      rrfStars,
	"sub": func(a, b int) int {
		return a - b
	},

	// Boolean functions
	"yesno": yesno,

	// URL functions
	"urlSetParam": urlSetParam,
	"urlDelParam": urlDelParam,

	// JSON functions
	"json": marshalJSON,

	// Dictionary functions
	"dict": func(values ...any) (map[string]any, error) {
		if len(values)%2 != 0 {
			return nil, fmt.Errorf("invalid dict call")
		}
		dict := make(map[string]any, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	},
}

func formatTime(format string, t time.Time) string {
	return t.Format(format)
}

func ApproxAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}

	d := time.Since(t)
	if d < time.Second {
		return "just now"
	}

	ds := int(math.Round(d.Seconds()))
	if ds < 60 {
		if ds == 1 {
			return "1 second ago"
		}
		if ds == 0 {
			return "just now"
		}
		return fmt.Sprintf("%d seconds ago", ds)
	}

	dm := int(math.Round(d.Minutes()))
	if dm < 60 {
		if dm == 1 {
			return "1 minute ago"
		} else if dm == 0 {
			return "just now"
		}
		return fmt.Sprintf("%d minutes ago", dm)
	}

	dh := int(math.Round(d.Hours()))
	if dh < 24 {
		if dh == 1 {
			return "1 hour ago"
		} else if dh == 0 {
			return "just now"
		}
		return fmt.Sprintf("%d hours ago", dh)
	}

	dd := int(math.Round(float64(d / day)))
	if dd < 365 {
		if dd == 1 {
			return "1 day ago"
		} else if dd == 0 {
			return "just now"
		}
		return fmt.Sprintf("%d days ago", dd)
	}

	dy := int(math.Round(float64(d / year)))
	if dy == 1 {
		return "1 year ago"
	} else if dy == 0 {
		return "just now"
	}
	return fmt.Sprintf("%d years ago", dy)
}

func ApproxDuration(d time.Duration) string {
	if d < time.Second {
		return "less than 1 second"
	}

	ds := int(math.Round(d.Seconds()))
	if ds == 1 {
		return "1 second"
	} else if ds < 60 {
		return fmt.Sprintf("%d seconds", ds)
	}

	dm := int(math.Round(d.Minutes()))
	if dm == 1 {
		return "1 minute"
	} else if dm < 60 {
		return fmt.Sprintf("%d minutes", dm)
	}

	dh := int(math.Round(d.Hours()))
	if dh == 1 {
		return "1 hour"
	} else if dh < 24 {
		return fmt.Sprintf("%d hours", dh)
	}

	dd := int(math.Round(float64(d / day)))
	if dd == 1 {
		return "1 day"
	} else if dd < 365 {
		return fmt.Sprintf("%d days", dd)
	}

	dy := int(math.Round(float64(d / year)))
	if dy == 1 {
		return "1 year"
	}

	return fmt.Sprintf("%d years", dy)
}

func pluralize(count any, singular string, plural string) (string, error) {
	n, err := toInt64(count)
	if err != nil {
		return "", err
	}

	if n == 1 {
		return singular, nil
	}

	return plural, nil
}

func Slugify(s string) string {
	var buf bytes.Buffer

	for _, r := range s {
		switch {
		case r > unicode.MaxASCII:
			continue
		case unicode.IsLetter(r):
			buf.WriteRune(unicode.ToLower(r))
		case unicode.IsDigit(r), r == '_', r == '-':
			buf.WriteRune(r)
		case unicode.IsSpace(r):
			buf.WriteRune('-')
		}
	}

	return buf.String()
}

func safeHTML(s string) template.HTML {
	return template.HTML(s)
}

func incr(i any) (int64, error) {
	n, err := toInt64(i)
	if err != nil {
		return 0, err
	}

	n++
	return n, nil
}

func decr(i any) (int64, error) {
	n, err := toInt64(i)
	if err != nil {
		return 0, err
	}

	n--
	return n, nil
}

func formatInt(i any) (string, error) {
	n, err := toInt64(i)
	if err != nil {
		return "", err
	}

	return printer.Sprintf("%d", n), nil
}

func formatFloat(f float64, dp int) string {
	format := "%." + strconv.Itoa(dp) + "f"
	return printer.Sprintf(format, f)
}

func yesno(b bool) string {
	if b {
		return "Yes"
	}

	return "No"
}

func urlSetParam(u *url.URL, key string, value any) *url.URL {
	nu := *u
	values := nu.Query()

	values.Set(key, fmt.Sprintf("%v", value))

	nu.RawQuery = values.Encode()
	return &nu
}

func urlDelParam(u *url.URL, key string) *url.URL {
	nu := *u
	values := nu.Query()

	values.Del(key)

	nu.RawQuery = values.Encode()
	return &nu
}

func toInt64(i any) (int64, error) {
	switch v := i.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	// Note: uint64 not supported due to risk of truncation.
	case string:
		return strconv.ParseInt(v, 10, 64)
	}

	return 0, fmt.Errorf("unable to convert type %T to int", i)
}

// mul multiplies two numeric values and returns a float64
func mul(a, b any) (float64, error) {
	aFloat, err := toFloat64(a)
	if err != nil {
		return 0, fmt.Errorf("first argument: %w", err)
	}

	bFloat, err := toFloat64(b)
	if err != nil {
		return 0, fmt.Errorf("second argument: %w", err)
	}

	return aFloat * bFloat, nil
}

// percentage converts a decimal value to a percentage string with specified decimal places
func percentage(value any, decimalPlaces int) (string, error) {
	f, err := toFloat64(value)
	if err != nil {
		return "", fmt.Errorf("unable to convert to percentage: %w", err)
	}

	// Convert to percentage (multiply by 100) and format
	percentage := f * 100
	format := "%." + strconv.Itoa(decimalPlaces) + "f%%"
	return printer.Sprintf(format, percentage), nil
}

// toFloat64 converts various numeric types to float64
func toFloat64(i any) (float64, error) {
	switch v := i.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	}

	return 0, fmt.Errorf("unable to convert type %T to float64", i)
}

// humanDate formats a time.Time into a human-readable date
func humanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("02 Jan 2006 at 15:04")
}

// marshalJSON converts a value to a JSON string for use in templates
func marshalJSON(v any) (template.JS, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return template.JS(b), nil
}

// rrfPercentage converts RRF + recency scores to 0-100% for user display
//
// Score ranges for different recency weight settings:
//
//	Recency Weight 0.0: 0.0328 - 0.0661 (pure RRF, no recency boost)
//	Recency Weight 0.1: 0.0295 - 0.1594 (light recency boost)
//	Recency Weight 0.2: 0.0262 - 0.2528 (default recency boost) ← CURRENT
//	Recency Weight 0.3: 0.0230 - 0.3461 (heavy recency boost)
//
// Current configuration uses recency weight 0.2 with half-life 30.0 days
func rrfPercentage(score float64) int {
	minScore := 0.0262 // Minimum: low RRF + no recency boost (recency weight 0.2)
	maxScore := 0.2528 // Maximum: high RRF + full recency boost (recency weight 0.2)

	if score <= minScore {
		return 0
	}
	if score >= maxScore {
		return 100
	}

	percentage := ((score - minScore) / (maxScore - minScore)) * 100
	return int(math.Round(percentage))
}

// rrfStars converts RRF + recency scores to star ratings for user display
//
// Uses the same score ranges as rrfPercentage for consistency:
//
//	Recency Weight 0.0: 0.0328 - 0.0661 (pure RRF, no recency boost)
//	Recency Weight 0.1: 0.0295 - 0.1594 (light recency boost)
//	Recency Weight 0.2: 0.0262 - 0.2528 (default recency boost) ← CURRENT
//	Recency Weight 0.3: 0.0230 - 0.3461 (heavy recency boost)
//
// Star thresholds based on percentage within current range:
//
//	★★★★★ (5 stars): 90-100% (scores 0.2302-0.2528 for weight 0.2)
//	★★★★☆ (4 stars): 70-89%  (scores 0.1844-0.2301 for weight 0.2)
//	★★★☆☆ (3 stars): 50-69%  (scores 0.1395-0.1843 for weight 0.2)
//	★★☆☆☆ (2 stars): 30-49%  (scores 0.0942-0.1394 for weight 0.2)
//	★☆☆☆☆ (1 star):  10-29%  (scores 0.0489-0.0941 for weight 0.2)
//	☆☆☆☆☆ (0 stars): 0-9%    (scores 0.0262-0.0488 for weight 0.2)
func rrfStars(score float64) string {
	// Use same range as rrfPercentage: 0.0262 to 0.2528
	minScore := 0.0262
	maxScore := 0.2528

	// Calculate percentage to determine star rating
	var percentage float64
	if score <= minScore {
		percentage = 0
	} else if score >= maxScore {
		percentage = 100
	} else {
		percentage = ((score - minScore) / (maxScore - minScore)) * 100
	}

	switch {
	case percentage >= 90: // 90-100%
		return "★★★★★"
	case percentage >= 70: // 70-89%
		return "★★★★☆"
	case percentage >= 50: // 50-69%
		return "★★★☆☆"
	case percentage >= 30: // 30-49%
		return "★★☆☆☆"
	case percentage >= 10: // 10-29%
		return "★☆☆☆☆"
	default: // 0-9%
		return "☆☆☆☆☆"
	}
}
