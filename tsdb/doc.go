package tsdb

/*


━━━━━━━━━━━━━━━━━━━━━━━━━━IO Flow━━━━━━━━━━━━━━━━━━━━━━━━

Each shard contains a MemoryDatabase, the Index Database is a global singleton

a) Write Flow

+-------------------------------------+
|                                     |
|               Engine                |
|                                     |
+------+----------------------+-------+
       |                      |
       |                      |
Shard  |               Shard  |
+------v-------+       +------v-------+
| Data | Memory|       | Data | Memory+----------------------------------------------------+
|  DB  |   DB  |       |  DB  |   DB  +--------------+                                     |
+-----^-+------+       +-----^-+------+              |                                     |
      | |                    | |                     |                                     |
      | |                    | | ID                  |                                     |
      | |                    | | Generator           |                                     |
+-----+-v--------------------+-v------+              |                                     |
|                                     |              |                                     |
|            ID Sequencer             |              |                                     |
|                                     |              +--------------+                      |
+------+----------------------+-------+              |              |                      |
       |                      |                      |              |                      |
       |                      |                      | SeriesIndex- | ForwardIndex-        |
       | NameIDIndexFlusher   | MetaIndexFlusher     | Flusher      | Flusher              | MetricDataFlusher
+------v-------+       +------v-------+       +------v-------+------v-------+       +------v-------+
| MetricNameID |       |  MetricMeta  |       |SeriesInverted| SeriesForward|       |  MetricData  |
|  IndexTable  |       |  IndexTable  |       |  IndexTable  |  IndexTable  |       |    Table     |
+--------------+       +--------------+       +--------------+--------------+       +--------------+


b) Query flow

Shard                  Shard
+------+-------+       +-----+--------+                Suggester
| Data | Memory|       | Data | Memory<--------------+ MetaGetter
|  DB  |   DB  |       |  DB  |   DB  |              | Filter
+-----^-+------+       +-----^-+------+              | DataGetter
      | |                    | |                     +----------------------
      | |                    | |
      | |           IDGetter | |
+-----+-v--------------------+-v------+
|                                     <----------------------------------------------------+
|            ID Sequencer             |                                                    |
|                                     <--------------+--------------+                      |
+------^----------------------^-------+              |              |                      |
       |                      |                      | Suggest-     |                      |
       ^ SuggestMetrics       ^ SuggestTagKeys       ^ TagValues    ^                      ^
       | NameIDIndexReader    | MetaIndexReader      | Filter       | MetaGetter           | DataGetter
+------+-------+       +------+-------+       +------+-------+------+-------+       +------+-------+
| MetricNameID |       |  MetricMeta  |       |SeriesInverted| SeriesForward|       |  MetricData  |
|  IndexTable  |       |  IndexTable  |       |  IndexTable  |  IndexTable  |       |    Table     |
+--------------+       +--------------+       +--------------+--------------+       +--------------+



━━━━━━━━━━━━━━━━━━━━━━━━━━Layout of MemoryDatabase━━━━━━━━━━━━━━━━━━━━━━━━

+--------------+       +--------------+
|              |------>|              |
|              |-+     |              |-+
|   Memory     | |     |  Metric      | |
|   Database   | |-+   |  Store       | |-+
|   RwMutex    | | |   |  RWMutex     | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
+-+------------+ | |   +--------------+ | |
  +--------------+ |     +-----|--------+ |
    +--------------+       +---|----------+
                               |
                               V
+--------------+       +--------------+
|              |<------|              |
|              |-+     |              |-+
|              | |     |              | |
|   Field      | |-+   |  TimeSeries  | |-+
|   Store      | | |   |  Store       | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |              | | |
|              | | |   |   SpinLock   | | |
+--------------+ | |   +--------------+ | |
  +----|---------+ |     +--------------+ |
    +--|-----------+       +--------------+
       |
       V
+--------------+
|              |
|              |-+
|              | |
|   Segment    | |-+
|   Store      | | |
|              | | |
|              | | |
|              | | |
|              | | |
|              | | |
+--------------+ | |
  +--------------+ |
    +--------------+

━━━━━━━━━━━━━━━━━━━━━━━Layout of Series Forward Index Table━━━━━━━━━━━━━━━━━━━━━━━━

                   Level1
                   +---------+---------+---------+---------+---------+---------+
                   | Metric  | Metric  | Metric  | Metric  | Metric  | Footer  |
                   | Block   | Block   | Block   | Offset  | Index   |         |
                   +---------+---------+---------+---------+---------+---------+
                  /           \                  |          \         \
                 /             \                 |           \         \
                /               \                +            \         +------+
               /                 \                \            \                \
  +-----------+                   +--------+       \            +--------+       \
 /                 Level2                   \       \                     \       \
v--------+--------+--------+--------+--------v       v--------+---+--------v-------v
| Version| Version| Version| Version| Footer |       | Offset |...| Offset | Metric|
| Entry1 | Entry2 | Entry3 | Offsets|        |       |        |   |        | Bitmap|
+--------+--------+--------+--------+--------+       +--------+---+--------+-------+
         |        |
         |        |
  +------+        +---------------------------------------------------+
 /                 Level3                                              \
v--------+--------+--------+--------+--------+--------+--------+--------v
|  Time  | TagKeys| Dict   |TagKeys | Series |Offsets |SeriesID| Footer |
|  Range | Block  | Block  |LUTBlock|LUTBlock| Block  | BitMap |        |
+--------+--------+--------+--------+--------+--------+--------+--------+

Level1(KV table: MetricID -> MetricBlock)
Level1 is same as MetricDataTable as below


Level2(Version Offsets Block)
┌────────────────────────────────┐┌──────────────────────────────────────────────────────┐┌─────────────────────┐
│          Version Entries       ││                     Version Offsets                  ││        Footer       │
├──────────┬──────────┬──────────┤├──────────┬──────────┬──────────┬──────────┬──────────┤├──────────┬──────────┤
│  Version │  Version │  Version ││ Versions │ Version1 │ Version1 │ Version2 │ Version2 ││VersionOff│ CRC32    │
│  Entry1  │  Entry2  │  Entry3  ││  Count   │  uint32  │  Length  │  uint32  │  Length  ││ setsPos  │ CheckSum │
├──────────┼──────────┼──────────┤├──────────┼──────────┼──────────┼──────────┼──────────┤├──────────┼──────────┤
│  N Bytes │  N Bytes │  N Bytes ││ uvariant │  4 Bytes │ uvariant │  4 Bytes │ uvariant ││ 4 Bytes  │ 4 Bytes  │
└──────────┴──────────┴──────────┘└──────────┴──────────┴──────────┴──────────┴──────────┘└──────────┴──────────┘


Level3(Version Entry Block)
TagKeysBlock stores all tagKeys of the metric
┌─────────────────────┐┌──────────────────────────────────────────────────────┐┌──────────┐┌─────────────────────┐
│  Time Range Block   ||                      TagKeys Block                   │|Dict Block||      Tags Blocks    │
├──────────┬──────────┤├──────────┬──────────┬──────────┬──────────┬──────────┤├──────────┤├──────────┬──────────┤
│   Start  │   End    │|  TagKey  │  TagKey1 |  TagKey1 │  TagKey2 │  TagKey2 │|          │|TagsBlock1│TagsBlock2│
│   Time   │   Time   ||  Count   │  Length  │          │  Length  │          │|  .....   │|          │          │
├──────────┼──────────┤├──────────┼──────────┼──────────┼──────────┼──────────┤├──────────┤├──────────┼──────────┤
│  4 Bytes │  4 Bytes │| uvariant │ uvariant │  N Bytes │ uvariant │  N Bytes │|  N Bytes │|  N Bytes │  N Bytes │
└──────────┴──────────┘└──────────┴──────────┴──────────┴──────────┴──────────┘└──────────┘^──────────^──────────┘
                                                                                           |          |
                                                                                         PosOfTags1 PosOfTags2


Level3(Dict Block)
Dict Block is composed of 2 parts:
1) String Block Offsets
   TagValues of the metric are split into multi string blocks(each block size is up to 400)

2) Snappy Compressed String Blocks
   Theoretically, one compressed string block may cost 1-3 pages(4KB/page)

┌───────────────────────────────────────────┐┌──────────┐┌───────────────────────────────────────────┐
│       Snappy Compressed String Block      │| StrBlocks|│             String Block Offsets          │
├──────────┬──────────┬──────────┬──────────┤├──────────┤├──────────┬──────────┬──────────┬──────────┤
│ TagValue1│ TagValue1│ TagValue2│ TagValue2│|  ....... ││  Strings │ StrBlock1│ StrBlock2│ StrBlock3│
│  Length  │          │  Length  │          │|          ││  Count   │  Length  │  Length  │  Length  │
├──────────┼──────────┼──────────┼──────────┤├──────────┤├──────────┼──────────┼──────────┼──────────┤
│ uvariant │  N Bytes │ uvariant │  N Bytes │|          ││ uvariant │ uvariant │ uvariant │ uvariant │
└──────────┴──────────┴──────────┴──────────┘└──────────┘^──────────┴──────────┴──────────┴──────────┘
 \____________________  ___________________/             |
                      \/                                 |
                StrBlock1Length                      PosOfDictBlockOffsets


Level3(TagKeys LOOKUP-TABLE Block)
This block provides a ability to filter tagValues by a specified tagKeys

┌────────────────────────────────────────────────────────────────────────────┐
│                          Keys LOOKUP-TABLE Block                           │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│  Key1    │Key1Values│ Value1Of │ Value2Of │  Key2    │Key2Values│ Value1Of │
│  Length  │   Count  │   Key1   │   Key1   │  Length  │   Count  │   Key2   │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ uvariant │ uvariant │ uvariant │ uvariant │ uvariant │ uvariant │
^──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘
|
PosOfKeysLUT


Level3(Series TagsKeyValue LOOKUP-TABLE Block)
SeriesTagsBlock is composed of 2 parts:
1) bit-array of tagKeys of this seriesID
   TagKeys Block stores all tagKeys of this seriesID,
   If there are 15 tagKeys of the metric, and this series is composed of the 1st, 3rd,5th, 14th,
   then the bit-array is 0101,0100,0000,0010. Offsets are listed in order after the bit-array,

2) tagValue offsets is used to index for the dict block
   each tagValue-index is uvariant encoded

┌──────────────────────────────────────────────────────┐
│             Series Tags LOOKUP-TABLE Block           │
├──────────┬──────────┬──────────┬──────────┬──────────┤
│ TagsKey  │ StrBlock │ StrBlock │ StrBlock │ StrBlock │
│ BitArray │ Sequence1│ Sequence2│ Sequence3│ Sequence4│
├──────────┼──────────┼──────────┼──────────┼──────────┤
│ N Bytes  │ uvariant │ uvariant │ uvariant │ uvariant │
└──────────┴──────────┴──────────┴──────────┴──────────┘


Level3(Footer)
┌───────────────────────────────────────────┐
│                   Footer                  │
├──────────┬──────────┬──────────┬──────────┤
│PosOfDictB│ PosOfKeys| PosOfOff │  PosOf   │
│lockOffset│   LUT    | setBlock │  BitMap  │
├──────────┼──────────┼──────────┼──────────┤
│ 4 Bytes  │ 4 Bytes  | 4 Bytes  │  4 Bytes │
└──────────┴──────────┴──────────┴──────────┘


━━━━━━━━━━━━━━━━━━━━━━━Layout of Series Inverted Index Table━━━━━━━━━━━━━━━━━━━━━━━━

                   Level1
                   +---------+---------+---------+---------+---------+---------+
                   |  TagKV  |  TagKV  |  TagKV  |  TagKV  |  TagKV  | Footer  |
                   | EntrySet| EntrySet| EntrySet| Offset  |  Index  |         |
                   +---------+---------+---------+---------+---------+---------+
                  /           \                   \        |\        +-------------------------------+
                 /             \                   \       | +--------------------------------+       \
                /               \                   \      +-----------------------------+     \       \
               /                 \                   +--------------+                     \     \       \
  +-----------+                   +-----------------+                \                     \     \       \
 /                 Level2                            \                \                     \     \       \
v--------+--------+--------+--------+--------+--------v                v--------+---+--------v     v-------v
|  Time  | LOUDS  |TagValue|TagValue|TagValue| CRC32  |                | Offset |...| Offset |     | TagKV |
|  Range |TrieTree|  Info  | Data1  | Data2  |CheckSum|                |        |   |        |     | Bitmap|
+--------+--------+--------+--------+--------+--------+                +--------+---+--------+     +-------+


Level1(KV table: TagKeyID -> EntrySetBlock)
Level1 is same as MetricDataTable as below
This block is alias as EntrySetBlock

Level2(TimeRange & LOUDS Encoded Trie Tree)
This block is alias as TreeBlock
┌─────────────────────┬────────────────────────────────────────────────────────────────────────────┐
│       TimeRange     │                        LOUDS Encoded Trie Tree                             │
├──────────┬──────────┼──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│ StartTime│  EndTime │   Trie   │  Labels  │  labels  │ isPrefix │ isPrefix │  LOUDS   │  LOUDS   │
│  uint32  │  uint32  │  TreeLen │  Length  │  Block   │ Key Len  │Key BitMap│  Length  │  BitMap  │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│  4 Bytes │  4 Bytes │ uvariant │ uvariant │ N Bytes  │ uvariant │ N Bytes  │ uvariant │ N Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘

Level2(TagValue Info)
alias as OffsetsBlock
┌────────────────────────────────┐
│          TagValue Info         │
├──────────┬──────────┬──────────┤
│ TagValue │  Data1   │  Data2   │
│  Count   │  Length  │  Length  │
├──────────┼──────────┼──────────┤
│ uvariant │ uvariant │ uvariant │
└──────────┴──────────┴──────────┘


Level2(Versioned TagValue Data)
alias as TagValueDataBlock
┌──────────┬──────────────────────────────────────────────────────┬─────────────────────┐
│          │                  VersionedTagValue                   │  VersionedTagValues │
├──────────┼──────────┬──────────┬──────────┬──────────┬──────────┼──────────┬──────────┤
│ Version  │ Version1 │StartTime1│ EndTime1 │ BitMap1  │ TagValue1│  Version │ Version  │
│  Count   │  uint32  │ (Delta)  │  (Delta) │  Length  │  BitMap  │   Meta2  │  Meta3   │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ 4 Bytes  │ variant  │ variant  │ uvariant │ N Bytes  │ N Bytes  │  N Bytes │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘

Succinct trie tree(Example):
(KEY Value: eleme:1, etcd:2, etrace:3)

Labels: eltecrmdaece
isPrefixKey: 0000000010101
LOUDS: 1011010110101010100100100
Values: [2, 1, 3]


                   +--------+
                   |        | (pseudo root)
                   |  10    | (node-0)
                   +--------+
                       |
                   +---v----+
                   |        | (root)
                   |  10    | (node-1)
                   +--------+
                       |
                   +---v----+
                   |   e    |
                   |  110   | (node-2)
                   +---+----+
                      / \
              +------+   +----+
             /                 \
        +---v----+          +---v----+
        |   l    |          |   t    |
        |   10   | (node-3) |   110  |(node-4)
        +---+----+          +---+----+
            |                   |\_______________
            |                   |                \
        +---v----+          +---v----+        +---v----+
        |   e    |          |   c    |        |   r    |
        |   10   | (node-5) |   10   |(node-6)|   10   | (node-7)
        +---+----+          +---+----+        +---+----+
            |                   |                 |
        +---v----+          +---v----+        +---v----+
        |   m    |          |   d    |        |   a    |
        |   10   | (node-8) |   0    |(node-9)|   10   | (node-10)
        +---+----+          +--------+        +---+----+
            |                 Value:2             |
        +---v----+                            +---v----+
        |   e    |                            |   c    |
        |   0    | (node-11)                  |   10   | (node-12)
        +--------+                            +---+----+
          Value:1                                 |
                                              +---v----+
                                              |   e    |
                                              |   0    | (node-13)
                                              +--------+
                                               Value:3


━━━━━━━━━━━━━━━━━━━━━━━Layout of Metric NameID Index Table━━━━━━━━━━━━━━━━━━━━━━━━
Metric-NameID-Table is a gzip compressed k/v pairs of metricNames and metricIDs on disk.

                   Level1
                   +---------+---------+---------+---------+
                   | Metric  |  Meta   | Index   | Footer  |
                   | KVPair  |         |         |         |
                   +---------+---------+---------+---------+

Level1(Metric NameID KVPair)
┌─────────────────────────────────────────────────────────────────┬─────────────────────┐
│            Gzip Compressed Metric K/V pairs                     │  SequenceNumber     │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┼──────────┬──────────┤
│MetricName│MetricName│ MetricID │MetricName│MetricName│ MetricID │ MetricID │  TagID   │
│  Length  │          │          │  Length  │          │          │ Sequence │ Sequence │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ N Bytes  │ 4 Bytes  │ uvariant │ N Bytes  │ 4 Bytes  │ 4 Bytes  │ 4 Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘


━━━━━━━━━━━━━━━━━━━━━━━Layout of Metric Meta Index Table━━━━━━━━━━━━━━━━━━━━━━━━
Metric-Meta stores meta info for metric,
such as tagKey, tagID, fieldID, fieldName and fieldType etc.

                   Level1
                   +---------+---------+---------+---------+---------+---------+
                   | Metric  | Metric  | Metric  | Metric  | Metric  | Footer  |
                   | Meta    |  Meta   |  Meta   |  Meta   | Index   |         |
                   +---------+---------+---------+---------+---------+---------+
                  /           \        |         |\        +--------------+
                 /             \       +         | +---------------+       \
                /               \       \        +------------+     \       \
               /                 \       \                     \     \       \
  +-----------+                   \       \                     \     \       \
 /                 Level2          \       \                     \     \       \
v--------+--------+--------+--------v       v--------+---+--------v     v-------v
|  Tag   | TagKey | Field  | Field  |       | Offset |...| Offset |     | Metric|
| MetaLen|  Meta  | MetaLen| Meta   |       |        |   |        |     | Bitmap|
+--------+--------+--------+--------+       +--------+---+--------+     +-------+

Level2(TagKey Meta)
┌──────────┬─────────────────────────────────────────────────────────────────┐
│  MetaLen │                       TagKey Meta                               │
├──────────┼──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│  TagKey  │  TagKey  │  TagKey  │  TagID   │  TagKey  │  TagKey  │  TagID   │
│  MetaLen │   Len    │          │          │   Len    │          │          │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │  1 Byte  │ N Bytes  │ 4 Bytes  │  1 Byte  │ N Bytes  │ 4 Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘

Level2(Field Meta)
┌──────────┬───────────────────────────────────────────────────────────────────────────────────────┐
│  MetaLen │                                    Field Meta                                         │
├──────────┼──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┤
│  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │  Field   │
│  MetaLen │   Len    │  Name    │  Type    │   ID     │   Len    │  Name    │  Type    │   ID     │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │  1 Byte  │ N Bytes  │ 1 Byte   │ 2 Bytes  │  1 Byte  │ N Bytes  │  1 Byte  │ 2 Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘


━━━━━━━━━━━━━━━━━━━━━━━━━━Layout of Metric Data Table━━━━━━━━━━━━━━━━━━━━━━

                   Level1
                   +---------+---------+---------+---------+---------+---------+
                   | Metric  | Metric  | Metric  | Metric  | Metric  | Footer  |
                   | Block   | Block   | Block   | Offset  | Index   |         |
                   +---------+---------+---------+---------+---------+---------+
                  /           \                   \        |\        +-------------------------------+
                 /             \                   \       | +--------------------------------+       \
                /               \                   \      +-----------------------------+     \       \
               /                 \                   +--------------+                     \     \       \
  +-----------+                   +--------------------------+       \                     \     \       \
 /                 Level2                                     \       \                     \     \       \
v--------+--------+--------+--------+--------+--------+--------v       v--------+---+--------v     v-------v
| Series | Series | Series | Series | Series | Fields | Footer |       | Offset |...| Offset |     | Metric|
| Entry  | Entry  | Entry  | Offset | Index  |  Meta  |        |       |        |   |        |     | Bitmap|
+--------+--------+--------+--------+--------+--------+--------+       +--------+---+--------+     +-------+
|         \                 \       |\        \
|          \                 \      | \        +-----------------------------------------------+
|           \                 \     |  +----------------------------------------------+         \
|            \                 \    +---------------------------------------------+    \         \
|             \                 +-----------------------------+                    \    \         \
|              +------------------------------------+          \                    \    \         \
|                  Level3                            \          \                    \    \         \
v--------+--------+--------+--------+--------+--------v          v--------+---+-------v    v---------v
| Fields | Data   |  Data  | Data   | Data   |  Data  |          | Offset |...| Offset|    |seriesID |
| Info   |        |        |        |        |        |          |        |   |       |    | Bitmap  |
+--------+--------+--------+--------+--------+--------+          +--------+---+-------+    +---------+


Level1(KV table: MetricBlocks, Offset, Keys)
┌───────────────────────────────────────────┬───────────────────────────────────────────┐
│               Metric Blocks               │           Offset And Keys                 │
├──────────┬──────────┬──────────┬──────────┼──────────┬──────────┬──────────┬──────────┤
│  length  │  Metric  │  length  │  Metric  │  length  │  Offset  │  length  │  Keys    │
│          │  Block1  │          │  Block2  │          │          │          │          │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │  N Bytes │ uvariant │ N Bytes  │ uvariant │  N Bytes │ uvariant │  N Bytes │
└──────────┴──────────┴──────────┴──────────^──────────┴──────────^──────────┴──────────┘
                                            |                     |
                                       posOfOffset             posOfKeys

Level1(KV table: Footer)
┌──────────────────────────────────────────────────────┐
│                    Footer                            │
├──────────┬──────────┬──────────┬──────────┬──────────┤
│  length  │ position │ position │  Table   │  Magic   │
│          │ OfOffset │ OfKeys   │ Version  │  Number  │
├──────────┼──────────┼──────────┼──────────┼──────────┤
│  1 Byte  │ 4 Bytes  │ 4 Bytes  │ 1 Bytes  │  8 Bytes │
└──────────┴──────────┴──────────┴──────────┴──────────┘



Level2(Fields Meta, Fields Footer)
┌─────────────────────────────────────────────────────────────────┬───────────────────────────────────────────┐
│               Fields Meta                                       │           Fields Footer                   │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┼──────────┬──────────┬──────────┬──────────┤
│StartTime │ EndTime  │ Count    │ FieldID  │  Field   │          │ OffsetOf │ OffsetOf │ OffsetOf │  CRC32   │
│ (delta)  │ (delta)  │          │ (uint16) │  Type    │  ......  │ TSOffset │ TSIndex  │FieldsMeta│ Checksum │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ uvariant │ uvariant │ 2 Bytes  │ 1 Byte   │ 3N Bytes │ 4 Bytes  │ 4 Bytes  │ 4 Bytes  │  4 Bytes │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘


Level3(Fields Info, Fields Data)
┌─────────────────────────────────────────────────────────────────┬─────────────────────┐
│               Fields Info                                       │   Fields Data       │
├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┼──────────┬──────────┤
│StartTime │ EndTime  │ BitArray │ BitArray │  Data1   │  Data2   │  Data1   │ Data2    │
│ (delta)  │ (delta)  │  Length  │          │  Length  │  Length  │          │          │
├──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┼──────────┤
│ uvariant │ uvariant │ uvariant │ N Bytes  │ uvariant │ uvariant │ N Bytes  │ N Bytes  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘
bit array example(10101001, 1010100110101001)


*/
