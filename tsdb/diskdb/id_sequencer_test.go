package diskdb

import (
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/lindb/lindb/kv"
	"github.com/lindb/lindb/kv/table"
	"github.com/lindb/lindb/kv/version"
	"github.com/lindb/lindb/tsdb/field"
	"github.com/lindb/lindb/tsdb/tblstore"

	"github.com/golang/mock/gomock"
	art "github.com/plar/go-adaptive-radix-tree"
	"github.com/stretchr/testify/assert"
)

// for test singleton
var (
	singletonLock sync.Mutex
)

type mockedIDSequencer struct {
	idSequencer *idSequencer
	family      *kv.MockFamily
	snapShot    *version.MockSnapshot
	reader      *table.MockReader
	flusher     *kv.MockFlusher
}

func (db *mockedIDSequencer) Clear() {
	db.idSequencer.tree = newArtTree()
	db.idSequencer.metricIDSequence = 0
	db.idSequencer.tagKeyIDSequence = 0
	db.idSequencer.youngMetricNameIDs = make(map[string]uint32)
	db.idSequencer.youngTagKeyIDs = make(map[uint32][]tagKeyAndID)
	db.idSequencer.youngFieldIDs = make(map[uint32][]fieldIDAndType)
}

func (db *mockedIDSequencer) WithFindReadersError() {
	db.snapShot.EXPECT().FindReaders(gomock.Any()).Return(nil, fmt.Errorf("error"))
}

func (db *mockedIDSequencer) WithFindReadersOK() {
	db.snapShot.EXPECT().FindReaders(gomock.Any()).Return([]table.Reader{db.reader}, nil)
}

func mockIDSequencer(ctrl *gomock.Controller) *mockedIDSequencer {
	mockReader := table.NewMockReader(ctrl)

	mockFlusher := kv.NewMockFlusher(ctrl)

	mockSnapShot := version.NewMockSnapshot(ctrl)
	mockSnapShot.EXPECT().Close().Return().AnyTimes()

	mockFamily := kv.NewMockFamily(ctrl)
	mockFamily.EXPECT().GetSnapshot().Return(mockSnapShot).AnyTimes()
	mockFamily.EXPECT().NewFlusher().Return(mockFlusher).AnyTimes()

	sequencer := NewIDSequencer(mockFamily, mockFamily).(*idSequencer)
	sequencer.metaFamily = mockFamily
	sequencer.nameIDsFamily = mockFamily
	return &mockedIDSequencer{
		idSequencer: sequencer,
		family:      mockFamily,
		snapShot:    mockSnapShot,
		flusher:     mockFlusher,
		reader:      mockReader}
}

func Test_NewIDSequencer_Recover(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()
	// case1: mock snapshot FindReaders error
	mocked.WithFindReadersError()
	assert.NotNil(t, mocked.idSequencer.Recover())
	// case2: mock read ns ok
	mocked.WithFindReadersOK()
	mocked.reader.EXPECT().Get(gomock.Any()).Return([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	assert.Nil(t, mocked.idSequencer.Recover())
	// case3: mock unmarshal error
	mocked.WithFindReadersOK()
	mocked.reader.EXPECT().Get(gomock.Any()).Return([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	assert.NotNil(t, mocked.idSequencer.Recover())
}

func Test_IDSequencer_SuggestMetrics(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()
	for i := 10000; i < 30000; i++ {
		mocked.idSequencer.tree.Insert(art.Key(strconv.Itoa(i)), i)
	}
	// case1: invalid limit
	assert.Len(t, mocked.idSequencer.SuggestMetrics("1", -1), 0)
	// case2: limit exceeds the limit
	assert.Len(t, mocked.idSequencer.SuggestMetrics("2", 20000), 10000)
	// case3: smaller than limit
	assert.Len(t, mocked.idSequencer.SuggestMetrics("2000", 5000), 11)
}

func Test_IDSequencer_SuggestTagKeys(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()
	// case1: invalid limit
	assert.Len(t, mocked.idSequencer.SuggestTagKeys("", "", -1), 0)
	// case2: metricID not found
	assert.Len(t, mocked.idSequencer.SuggestTagKeys("", "", 100), 0)
	// case3: snapshot FindReaders error
	mocked.WithFindReadersError()
	mocked.idSequencer.tree.Insert([]byte("a"), uint32(1))
	assert.Len(t, mocked.idSequencer.SuggestTagKeys("a", "", 100), 0)
	// case4: snapshot FindReaders ok
	mocked.WithFindReadersOK()
	mocked.idSequencer.tree.Insert([]byte("a"), uint32(1))
	mocked.reader.EXPECT().Get(gomock.Any()).Return(nil)
	assert.Len(t, mocked.idSequencer.SuggestTagKeys("a", "", 100), 0)
}

func Test_IDSequencer_GetMetricID(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	// case1: neither in the map or on the tree
	metricID, err := mocked.idSequencer.GetMetricID("docker")
	assert.Zero(t, metricID)
	assert.NotNil(t, err)
	// in map
	mocked.idSequencer.youngMetricNameIDs["docker"] = 2
	metricID, err = mocked.idSequencer.GetMetricID("docker")
	assert.Equal(t, uint32(2), metricID)
	assert.Nil(t, err)
	// on the tree
	mocked.idSequencer.tree.Insert([]byte("cpu"), uint32(1))
	metricID, err = mocked.idSequencer.GetMetricID("cpu")
	assert.Equal(t, uint32(1), metricID)
	assert.Nil(t, err)
}

func Test_IDSequencer_GenMetricID(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()
	// newly created
	mocked.idSequencer.metricIDSequence = 2
	mocked.idSequencer.youngMetricNameIDs = map[string]uint32{"docker": 2}
	assert.Equal(t, uint32(2), mocked.idSequencer.GenMetricID("docker"))
	// metricID sequence
	assert.Equal(t, uint32(3), mocked.idSequencer.GenMetricID("cpu"))
	assert.Equal(t, uint32(3), mocked.idSequencer.GenMetricID("cpu"))
	assert.Equal(t, uint32(4), mocked.idSequencer.GenMetricID("cpu1"))
	assert.Equal(t, uint32(5), mocked.idSequencer.GenMetricID("cpu2"))
}

func Test_IDSequencer_GetTagID(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()
	// case1: tagKeyID exist in memory
	mocked.idSequencer.youngTagKeyIDs[uint32(1)] = []tagKeyAndID{{tagKey: "key", tagKeyID: uint32(2)}}
	tagID, err := mocked.idSequencer.GetTagID(1, "key")
	assert.Nil(t, err)
	assert.Equal(t, tagID, uint32(2))
	// case2: snapShot FindReaders error
	mocked.WithFindReadersError()
	_, err = mocked.idSequencer.GetTagID(1, "key2")
	assert.NotNil(t, err)
	// case3: snapShot FindReaders ok
	mocked.WithFindReadersOK()
	mocked.reader.EXPECT().Get(gomock.Any()).Return(nil)
	_, err = mocked.idSequencer.GetTagID(1, "key3")
	assert.NotNil(t, err)

	///////////////////////////////////
	// readTagID
	///////////////////////////////////
	mockMetaReader := tblstore.NewMockMetricsMetaReader(ctrl)
	// mock exist
	mockMetaReader.EXPECT().ReadTagID(gomock.Any(), gomock.Any()).Return(uint32(1), true)
	tagID, err = mocked.idSequencer.readTagID(mockMetaReader, 1, "")
	assert.Nil(t, err)
	assert.Equal(t, uint32(1), tagID)
	// mock not exist
	mockMetaReader.EXPECT().ReadTagID(gomock.Any(), gomock.Any()).Return(uint32(2), false)
	tagID, err = mocked.idSequencer.readTagID(mockMetaReader, 2, "")
	assert.NotNil(t, err)
	assert.Zero(t, tagID)
}

func Test_IDSequencer_GenTagID(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()

	// case1: tagKeyID exist in memory
	mocked.idSequencer.youngTagKeyIDs[uint32(5)] = []tagKeyAndID{{tagKey: "key", tagKeyID: uint32(2)}}
	tagID := mocked.idSequencer.GenTagID(5, "key")
	assert.Equal(t, tagID, uint32(2))
	// case2: snapShot FindReaders ok
	mocked.WithFindReadersOK()
	mocked.reader.EXPECT().Get(gomock.Any()).Return(nil)
	tagID = mocked.idSequencer.GenTagID(6, "key3")
	assert.Equal(t, uint32(1), tagID)
	// case3: exist in memory
	mocked.WithFindReadersOK()
	mocked.reader.EXPECT().Get(gomock.Any()).Return(nil)
	tagID = mocked.idSequencer.GenTagID(6, "key4")
	assert.Equal(t, uint32(2), tagID)
}

func Test_IDSequencer_GetFieldID(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()

	// case1: snapShot FindReaders error
	mocked.WithFindReadersError()
	fieldID, fieldType, err := mocked.idSequencer.GetFieldID(1, "f1")
	assert.Zero(t, fieldID)
	assert.Zero(t, fieldType)
	assert.NotNil(t, err)
	// case2: snapShot FindReaders ok
	mocked.WithFindReadersOK()
	mocked.reader.EXPECT().Get(gomock.Any()).Return(nil)
	_, _, err = mocked.idSequencer.GetFieldID(1, "f1")
	assert.NotNil(t, err)
	// case3: read existed fieldID
	mocked.idSequencer.youngFieldIDs = map[uint32][]fieldIDAndType{3: {{
		fieldType: field.SumField, fieldID: 1, fieldName: "sum"}}}
	fid, ftype, err := mocked.idSequencer.GetFieldID(3, "sum")
	assert.Nil(t, err)
	assert.Equal(t, uint16(1), fid)
	assert.Equal(t, field.SumField, ftype)

	///////////////////////////////////
	// readFieldID
	///////////////////////////////////
	mockMetaReader := tblstore.NewMockMetricsMetaReader(ctrl)
	// mock ok
	mockMetaReader.EXPECT().ReadFieldID(gomock.Any(), gomock.Any()).Return(
		uint16(1), field.SumField, true)
	fieldID, fieldType, err = mocked.idSequencer.readFieldID(mockMetaReader, 1, "f1")
	assert.Nil(t, err)
	assert.Equal(t, uint16(1), fieldID)
	assert.Equal(t, field.SumField, fieldType)
	// mock not exist
	mockMetaReader.EXPECT().ReadFieldID(gomock.Any(), gomock.Any()).
		Return(uint16(0), field.Type(0), false)
	fieldID, fieldType, err = mocked.idSequencer.readFieldID(mockMetaReader, 1, "cpu")
	assert.NotNil(t, err)
	assert.Zero(t, fieldID)
	assert.Zero(t, fieldType)
}

func Test_IndexDatabase_GenFieldID(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()

	// case1: hit memory, type match
	mocked.idSequencer.youngFieldIDs[1] = append(mocked.idSequencer.youngFieldIDs[1], fieldIDAndType{
		fieldID: 1, fieldType: field.SumField, fieldName: "sum"})
	fieldID, err := mocked.idSequencer.GenFieldID(1, "sum", field.SumField)
	assert.Equal(t, uint16(1), fieldID)
	assert.Nil(t, err)
	// case2: hit memory, type mismatch
	fieldID, err = mocked.idSequencer.GenFieldID(1, "sum", field.MinField)
	assert.Equal(t, uint16(0), fieldID)
	assert.NotNil(t, err)
	// case3: snapshot findReaders error
	mocked.WithFindReadersError()
	fieldID, err = mocked.idSequencer.GenFieldID(2, "sum", field.SumField)
	assert.Equal(t, uint16(0), fieldID)
	assert.NotNil(t, err)
	// case4: snapshot findReaders ok
	mocked.WithFindReadersOK()
	mocked.reader.EXPECT().Get(gomock.Any()).Return(nil).Times(2)
	fieldID, err = mocked.idSequencer.GenFieldID(3, "sum", field.SumField)
	assert.Equal(t, uint16(1), fieldID)
	assert.Nil(t, err)
	///////////////////////////////////
	// genFieldID
	///////////////////////////////////
	// case5: hit disk, type match
	mockMetaReader := tblstore.NewMockMetricsMetaReader(ctrl)
	mockMetaReader.EXPECT().ReadFieldID(gomock.Any(), gomock.Any()).
		Return(uint16(2), field.MinField, true).Times(2)
	fieldID, err = mocked.idSequencer.genFieldID(mockMetaReader, 1, "min", field.MinField)
	assert.Equal(t, uint16(2), fieldID)
	assert.Nil(t, err)
	// case6: hit disk, type mismatch
	fieldID, err = mocked.idSequencer.genFieldID(mockMetaReader, 1, "min", field.MaxField)
	assert.Zero(t, fieldID)
	assert.NotNil(t, err)

	// case7: new field, create ok
	mockMetaReader.EXPECT().ReadMaxFieldID(gomock.Any()).Return(uint16(2)).Times(2)
	mockMetaReader.EXPECT().ReadFieldID(gomock.Any(), gomock.Any()).
		Return(uint16(0), field.Type(0), false).AnyTimes()
	fieldID, err = mocked.idSequencer.genFieldID(mockMetaReader, 4, "sum", field.SumField)
	assert.Equal(t, uint16(3), fieldID)
	assert.Nil(t, err)
	fieldID, err = mocked.idSequencer.genFieldID(mockMetaReader, 4, "sum1", field.SumField)
	assert.Equal(t, uint16(4), fieldID)
	assert.Nil(t, err)
	// case8: new field, too many fields
	mockMetaReader.EXPECT().ReadMaxFieldID(gomock.Any()).Return(uint16(2000)).Times(1)
	fieldID, err = mocked.idSequencer.genFieldID(mockMetaReader, 5, "sum2", field.SumField)
	assert.Zero(t, fieldID)
	assert.NotNil(t, err)
}

func Test_IDSequencer_FlushNameIDs_FlushMetricsMeta(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()

	mocked.flusher.EXPECT().Add(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mocked.flusher.EXPECT().Commit().Return(fmt.Errorf("error")).AnyTimes()
	assert.NotNil(t, mocked.idSequencer.FlushNameIDs())
	assert.NotNil(t, mocked.idSequencer.FlushMetricsMeta())
}

func Test_IDSequencer_flushNameIDsTo(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()

	mockKVFlusher := kv.NewMockFlusher(ctrl)
	mockKVFlusher.EXPECT().Commit().Return(nil).AnyTimes()
	mockKVFlusher.EXPECT().Add(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mockFlusher := tblstore.NewMetricsNameIDFlusher(mockKVFlusher)
	assert.Nil(t, mocked.idSequencer.flushNameIDsTo(mockFlusher))

	mocked.idSequencer.youngMetricNameIDs["1"] = 1
	mocked.idSequencer.youngMetricNameIDs["2"] = 2
	mocked.idSequencer.metricIDSequence = 10
	mocked.idSequencer.tagKeyIDSequence = 15

	assert.Nil(t, mocked.idSequencer.flushNameIDsTo(mockFlusher))
	assert.Equal(t, 2, mocked.idSequencer.tree.Size())
	// mock add error
	mockKVFlusher.EXPECT().Add(gomock.Any(), gomock.Any()).Return(fmt.Errorf("error"))
	assert.NotNil(t, mocked.idSequencer.flushNameIDsTo(mockFlusher))

}

func Test_IDSequencer_flushMetricsMetaTo(t *testing.T) {
	singletonLock.Lock()
	defer singletonLock.Unlock()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mocked := mockIDSequencer(ctrl)
	mocked.Clear()

	set := func() {
		mocked.idSequencer.youngTagKeyIDs = map[uint32][]tagKeyAndID{
			1: {{tagKey: "11", tagKeyID: 11},
				{tagKey: "12", tagKeyID: 12}},
			2: {{tagKey: "22", tagKeyID: 22},
				{tagKey: "23", tagKeyID: 23}}}
		mocked.idSequencer.youngFieldIDs = map[uint32][]fieldIDAndType{
			2: {{fieldID: 22, fieldType: field.SumField},
				{fieldID: 23, fieldType: field.MaxField}},
			3: {{fieldID: 33, fieldType: field.MinField},
				{fieldID: 34, fieldType: field.SumField}}}
	}
	mockKVFlusher := kv.NewMockFlusher(ctrl)
	mockKVFlusher.EXPECT().Commit().Return(nil).AnyTimes()
	set()
	mockKVFlusher.EXPECT().Add(gomock.Any(), gomock.Any()).Return(nil).Times(3)
	mockMetaFlusher := tblstore.NewMetricsMetaFlusher(mockKVFlusher)
	assert.Nil(t, mocked.idSequencer.flushMetricsMetaTo(mockMetaFlusher))

	// map empty
	mockKVFlusher.EXPECT().Add(gomock.Any(), gomock.Any()).Return(fmt.Errorf("error")).Times(1)
	assert.Nil(t, mocked.idSequencer.flushMetricsMetaTo(mockMetaFlusher))
	// flush with error
	set()
	assert.NotNil(t, mocked.idSequencer.flushMetricsMetaTo(mockMetaFlusher))
}
