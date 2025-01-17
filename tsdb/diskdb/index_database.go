package diskdb

import (
	"github.com/lindb/lindb/constants"
	"github.com/lindb/lindb/kv"
	"github.com/lindb/lindb/pkg/timeutil"
	"github.com/lindb/lindb/sql/stmt"
	"github.com/lindb/lindb/tsdb/series"
	"github.com/lindb/lindb/tsdb/tblstore"
)

// indexDatabase implements IndexDatabase
type indexDatabase struct {
	idGetter            IDGetter
	invertedIndexFamily kv.Family
	forwardIndexFamily  kv.Family
}

// NewIndexDatabase returns a new IndexDatabase
func NewIndexDatabase(idGetter IDGetter, invertedIndexFamily, forwardIndexFamily kv.Family) IndexDatabase {
	return &indexDatabase{
		idGetter:            idGetter,
		invertedIndexFamily: invertedIndexFamily,
		forwardIndexFamily:  forwardIndexFamily}
}

// SuggestTagValues returns suggestions from given metricName, tagKey and prefix of tagValue
func (db *indexDatabase) SuggestTagValues(metricName, tagKey, tagValuePrefix string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if limit > constants.MaxSuggestions {
		limit = constants.MaxSuggestions
	}
	metricID, err := db.idGetter.GetMetricID(metricName)
	if err != nil {
		return nil
	}
	tagID, err := db.idGetter.GetTagID(metricID, tagKey)
	if err != nil {
		return nil
	}
	snapShot := db.invertedIndexFamily.GetSnapshot()
	defer snapShot.Close()

	readers, err := snapShot.FindReaders(metricID)
	if err != nil {
		return nil
	}
	return tblstore.NewInvertedIndexReader(readers).SuggestTagValues(tagID, tagValuePrefix, limit)
}

// GetTagValues get tag values corresponding with the tagKeys
func (db *indexDatabase) GetTagValues(metricID uint32, tagKeys []string, version uint32) (
	tagValues [][]string, err error) {

	snapShot := db.invertedIndexFamily.GetSnapshot()
	defer snapShot.Close()
	readers, err := snapShot.FindReaders(metricID)
	if err != nil {
		return nil, err
	}
	return tblstore.NewForwardIndexReader(readers).GetTagValues(metricID, tagKeys, version)
}

// FindSeriesIDsByExpr finds series ids by tag filter expr for metric id
func (db *indexDatabase) FindSeriesIDsByExpr(metricID uint32, expr stmt.TagFilter,
	timeRange timeutil.TimeRange) (*series.MultiVerSeriesIDSet, error) {
	tagID, err := db.idGetter.GetTagID(metricID, expr.TagKey())
	if err != nil {
		return nil, err
	}
	snapShot := db.invertedIndexFamily.GetSnapshot()
	defer snapShot.Close()

	readers, err := snapShot.FindReaders(metricID)
	if err != nil {
		return nil, err
	}
	return tblstore.NewInvertedIndexReader(readers).FindSeriesIDsByExprForTagID(tagID, expr, timeRange)
}

// GetSeriesIDsForTag get series ids for spec metric's tag key
func (db *indexDatabase) GetSeriesIDsForTag(metricID uint32, tagKey string,
	timeRange timeutil.TimeRange) (*series.MultiVerSeriesIDSet, error) {
	tagID, err := db.idGetter.GetTagID(metricID, tagKey)
	if err != nil {
		return nil, err
	}
	snapShot := db.invertedIndexFamily.GetSnapshot()
	defer snapShot.Close()

	readers, err := snapShot.FindReaders(metricID)
	if err != nil {
		return nil, err
	}
	return tblstore.NewInvertedIndexReader(readers).GetSeriesIDsForTagID(tagID, timeRange)
}
