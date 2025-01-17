package series

import (
	"io"

	"github.com/lindb/lindb/tsdb/field"
)

//go:generate mockgen -source ./iterator.go -destination=./iterator_mock.go -package=series

// VersionIterator represents a multi-version iterator
type VersionIterator interface {
	// Version returns the version no.
	Version() uint32
	// HasNext returns if the iteration has more time-series's iterator
	HasNext() bool
	// Next returns the time-series's iterator
	Next() Iterator
	// Close closes the underlying resource
	io.Closer
}

// GroupedIterator represents a iterator for the grouped time series data
type GroupedIterator interface {
	Iterator
	// Tags returns group tags
	Tags() map[string]string
}

// Iterator represents an iterator for the time series data
type Iterator interface {
	// HasNext returns if the iteration has more field's iterator
	HasNext() bool
	// Next returns the field's iterator
	Next() FieldIterator
	// SeriesID returns the time series id under current metric
	SeriesID() uint32
}

// FieldIterator represents a field's data iterator, support multi field for one series
type FieldIterator interface {
	// FieldID returns the field's id
	FieldID() uint16
	// FieldName return the field's name
	FieldName() string
	// FieldType returns the field's type
	FieldType() field.Type
	// HasNext returns if the iteration has more fields
	HasNext() bool
	// Next returns the primitive field iterator
	// because there are some primitive fields if field type is complex
	Next() PrimitiveIterator
}

// PrimitiveIterator represents an iterator over a primitive field, iterator points data of primitive field
type PrimitiveIterator interface {
	// FieldID returns the primitive field id
	FieldID() uint16
	// HasNext returns if the iteration has more data points
	HasNext() bool
	// Next returns the data point in the iteration
	Next() (timeSlot int, value float64)
}
