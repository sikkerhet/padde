CREATE TABLE padde.log
(
        query           String,
        answer          String,
        qtype           LowCardinality(String),
        first           SimpleAggregateFunction(min, UInt64),
        last            SimpleAggregateFunction(max, UInt64),
        count           SimpleAggregateFunction(sum, UInt64)
)
ENGINE = AggregatingMergeTree 
ORDER BY (query, answer, qtype);
