line=$(grep -n -m 1 "L1_INFO_TREE_UPDATES" ./tables.go | cut -d: -f1)
sed -i "$line a\L1_INFO_TREE_UPDATES_BY_GER=\"l1_info_tree_updates_by_ger\"" ./tables.go
line=$(grep -n -m 1 "BLOCK_L1_INFO_TREE_INDEX" ./tables.go | cut -d: -f1)
sed -i "$line a\BLOCK_L1_INFO_TREE_INDEX_PROGRESS=\"block_l1_info_tree_progress\"" ./tables.go

line=$(grep -n -m 1 "BATCH_COUNTERS" ./tables.go | cut -d: -f1)
sed -i "$line a\L1_BATCH_DATA=\"l1_batch_data\"" ./tables.go
line=$(grep -n -m 1 "L1_BATCH_DATA" ./tables.go | cut -d: -f1)
sed -i "$line a\REUSED_L1_INFO_TREE_INDEX=\"reused_l1_info_tree_index\"" ./tables.go
line=$(grep -n -m 1 "REUSED_L1_INFO_TREE_INDEX" ./tables.go | cut -d: -f1)
sed -i "$line a\LATEST_USED_GER=\"latest_used_ger\"" ./tables.go
line=$(grep -n -m 1 "LATEST_USED_GER" ./tables.go | cut -d: -f1)
sed -i "$line a\BATCH_BLOCKS=\"batch_blocks\"" ./tables.go
line=$(grep -n -m 1 "BATCH_BLOCKS" ./tables.go | cut -d: -f1)
sed -i "$line a\SMT_DEPTHS=\"smt_depths\"" ./tables.go
line=$(grep -n -m 1 "SMT_DEPTHS" ./tables.go | cut -d: -f1)
sed -i "$line a\L1_INFO_LEAVES=\"l1_info_leaves\"" ./tables.go
line=$(grep -n -m 1 "L1_INFO_LEAVES" ./tables.go | cut -d: -f1)
sed -i "$line a\L1_INFO_ROOTS=\"l1_info_roots\"" ./tables.go
line=$(grep -n -m 1 "L1_INFO_ROOTS" ./tables.go | cut -d: -f1)
sed -i "$line a\INVALID_BATCHES=\"invalid_batches\"" ./tables.go
line=$(grep -n -m 1 "INVALID_BATCHES" ./tables.go | cut -d: -f1)
sed -i "$line a\BATCH_PARTIALLY_PROCESSED=\"batch_partially_processed\"" ./tables.go
line=$(grep -n -m 1 "BATCH_PARTIALLY_PROCESSED" ./tables.go | cut -d: -f1)
sed -i "$line a\LOCAL_EXIT_ROOTS=\"local_exit_roots\"" ./tables.go
line=$(grep -n -m 1 "LOCAL_EXIT_ROOTS" ./tables.go | cut -d: -f1)
sed -i "$line a\ROllUP_TYPES_FORKS=\"rollup_types_forks\"" ./tables.go
line=$(grep -n -m 1 "ROllUP_TYPES_FORKS" ./tables.go | cut -d: -f1)
sed -i "$line a\FORK_HISTORY=\"fork_history\"" ./tables.go
line=$(grep -n -m 1 "FORK_HISTORY" ./tables.go | cut -d: -f1)
sed -i "$line a\JUST_UNWOUND=\"just_unwound\"" ./tables.go
line=$(grep -n -m 1 "JUST_UNWOUND" ./tables.go | cut -d: -f1)
sed -i "$line a\PLAIN_STATE_VERSION=\"plain_state_version\"" ./tables.go
line=$(grep -n -m 1 "PLAIN_STATE_VERSION" ./tables.go | cut -d: -f1)
sed -i "$line a\ERIGON_VERSIONS=\"erigon_versions\"" ./tables.go

line=$(grep -n -m 1 "ERIGON_VERSIONS" ./tables.go | cut -d: -f1)
sed -i "$line a\TableSmt=\"HermezSmt\"" ./tables.go
line=$(grep -n -m 1 "TableSmt" ./tables.go | cut -d: -f1)
sed -i "$line a\TableStats=\"HermezSmtStats\"" ./tables.go
line=$(grep -n -m 1 "TableStats" ./tables.go | cut -d: -f1)
sed -i "$line a\TableAccountValues=\"HermezSmtAccountValues\"" ./tables.go
line=$(grep -n -m 1 "TableAccountValues" ./tables.go | cut -d: -f1)
sed -i "$line a\TableMetadata=\"HermezSmtMetadata\"" ./tables.go
line=$(grep -n -m 1 "TableMetadata" ./tables.go | cut -d: -f1)
sed -i "$line a\TableHashKey=\"HermezSmtHashKey\"" ./tables.go

line=$(grep -n -m 1 "TableHashKey" ./tables.go | cut -d: -f1)
sed -i "$line a\TablePoolLimbo=\"PoolLimbo\"" ./tables.go



line=$(grep -n -m 1 "L1_INFO_TREE_UPDATES," ./tables.go | cut -d: -f1)
sed -i "$line a\L1_INFO_TREE_UPDATES_BY_GER," ./tables.go
line=$(grep -n -m 1 "BLOCK_L1_INFO_TREE_INDEX," ./tables.go | cut -d: -f1)
sed -i "$line a\BLOCK_L1_INFO_TREE_INDEX_PROGRESS," ./tables.go

line=$(grep -n -m 1 "BATCH_COUNTERS," ./tables.go | cut -d: -f1)
sed -i "$line a\L1_BATCH_DATA," ./tables.go
line=$(grep -n -m 1 "L1_BATCH_DATA," ./tables.go | cut -d: -f1)
sed -i "$line a\REUSED_L1_INFO_TREE_INDEX," ./tables.go
line=$(grep -n -m 1 "REUSED_L1_INFO_TREE_INDEX," ./tables.go | cut -d: -f1)
sed -i "$line a\LATEST_USED_GER," ./tables.go
line=$(grep -n -m 1 "LATEST_USED_GER," ./tables.go | cut -d: -f1)
sed -i "$line a\BATCH_BLOCKS," ./tables.go
line=$(grep -n -m 1 "BATCH_BLOCKS," ./tables.go | cut -d: -f1)
sed -i "$line a\SMT_DEPTHS," ./tables.go
line=$(grep -n -m 1 "SMT_DEPTHS," ./tables.go | cut -d: -f1)
sed -i "$line a\L1_INFO_LEAVES," ./tables.go
line=$(grep -n -m 1 "L1_INFO_LEAVES," ./tables.go | cut -d: -f1)
sed -i "$line a\L1_INFO_ROOTS," ./tables.go
line=$(grep -n -m 1 "L1_INFO_ROOTS," ./tables.go | cut -d: -f1)
sed -i "$line a\INVALID_BATCHES," ./tables.go
line=$(grep -n -m 1 "INVALID_BATCHES," ./tables.go | cut -d: -f1)
sed -i "$line a\BATCH_PARTIALLY_PROCESSED," ./tables.go
line=$(grep -n -m 1 "BATCH_PARTIALLY_PROCESSED," ./tables.go | cut -d: -f1)
sed -i "$line a\LOCAL_EXIT_ROOTS," ./tables.go
line=$(grep -n -m 1 "LOCAL_EXIT_ROOTS," ./tables.go | cut -d: -f1)
sed -i "$line a\ROllUP_TYPES_FORKS," ./tables.go
line=$(grep -n -m 1 "ROllUP_TYPES_FORKS," ./tables.go | cut -d: -f1)
sed -i "$line a\FORK_HISTORY," ./tables.go
line=$(grep -n -m 1 "FORK_HISTORY," ./tables.go | cut -d: -f1)
sed -i "$line a\JUST_UNWOUND," ./tables.go
line=$(grep -n -m 1 "JUST_UNWOUND," ./tables.go | cut -d: -f1)
sed -i "$line a\PLAIN_STATE_VERSION," ./tables.go
line=$(grep -n -m 1 "PLAIN_STATE_VERSION," ./tables.go | cut -d: -f1)
sed -i "$line a\ERIGON_VERSIONS," ./tables.go

line=$(grep -n -m 1 "ERIGON_VERSIONS," ./tables.go | cut -d: -f1)
sed -i "$line a\TableSmt," ./tables.go
line=$(grep -n -m 1 "TableSmt," ./tables.go | cut -d: -f1)
sed -i "$line a\TableStats," ./tables.go
line=$(grep -n -m 1 "TableStats," ./tables.go | cut -d: -f1)
sed -i "$line a\TableAccountValues," ./tables.go
line=$(grep -n -m 1 "TableAccountValues," ./tables.go | cut -d: -f1)
sed -i "$line a\TableMetadata," ./tables.go
line=$(grep -n -m 1 "TableMetadata," ./tables.go | cut -d: -f1)
sed -i "$line a\TableHashKey," ./tables.go

line=$(grep -n -m 1 "TableHashKey," ./tables.go | cut -d: -f1)
sed -i "$line a\TablePoolLimbo," ./tables.go
