package stmt

import (
	"encoding/json"

	"github.com/lindb/lindb/pkg/encoding"
	"github.com/lindb/lindb/pkg/interval"
	"github.com/lindb/lindb/pkg/timeutil"
)

// Query represents search statement
type Query struct {
	MetricName  string // like table name
	SelectItems []Expr // select list, such as field, function call, math expression etc.
	Condition   Expr   // tag filter condition expression

	TimeRange    timeutil.TimeRange // query time range
	Interval     int64              // down sampling interval
	IntervalType interval.Type      // interval type calc based on down sampling interval

	GroupBy []string // group by
	Limit   int      // num. of time series list for result
}

// innerQuery represents a wrapper of query for json encoding
type innerQuery struct {
	MetricName  string            `json:"metricName"`
	SelectItems []json.RawMessage `json:"selectItems"`
	Condition   json.RawMessage   `json:"condition"`

	TimeRange    timeutil.TimeRange `json:"timeRange"`
	Interval     int64              `json:"interval"`
	IntervalType interval.Type      `json:"intervalType"`

	GroupBy []string `json:"groupBy"`
	Limit   int      `json:"limit"`
}

// MarshalJSON returns json data of query
func (q *Query) MarshalJSON() ([]byte, error) {
	inner := innerQuery{
		MetricName:   q.MetricName,
		Condition:    Marshal(q.Condition),
		TimeRange:    q.TimeRange,
		Interval:     q.Interval,
		IntervalType: q.IntervalType,
		GroupBy:      q.GroupBy,
		Limit:        q.Limit,
	}
	for _, item := range q.SelectItems {
		inner.SelectItems = append(inner.SelectItems, Marshal(item))
	}
	return encoding.JSONMarshal(&inner), nil
}

// UnmarshalJSON parses json data to query
func (q *Query) UnmarshalJSON(value []byte) error {
	inner := innerQuery{}
	if err := encoding.JSONUnmarshal(value, &inner); err != nil {
		return err
	}
	if inner.Condition != nil {
		condition, err := Unmarshal(inner.Condition)
		if err != nil {
			return err
		}
		q.Condition = condition
	}
	var selectItems []Expr
	for _, item := range inner.SelectItems {
		selectItem, err := Unmarshal(item)
		if err != nil {
			return err
		}
		selectItems = append(selectItems, selectItem)
	}
	q.MetricName = inner.MetricName
	q.SelectItems = selectItems
	q.TimeRange = inner.TimeRange
	q.IntervalType = inner.IntervalType
	q.Interval = inner.Interval
	q.GroupBy = inner.GroupBy
	q.Limit = inner.Limit
	return nil
}
