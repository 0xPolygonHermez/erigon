#!/bin/bash

# steps:
# 1. run to where we will unwind to
# 2. dump the data
# 3. run to the final stop block
# 4. dump the data
# 5. unwind
# 6. dump the data
# 7. sync again to the final block
# 8. dump the data
# 9. compare the dumps at the unwind level and tip level

dataPath="./datadir"
firstStop=11204
stopBlock=11315
unwindBatch=70

rm -rf "$dataPath/rpc-datadir"
rm -rf "$dataPath/phase1-dump1"
rm -rf "$dataPath/phase1-dump2"
rm -rf "$dataPath/phase2-dump1"
rm -rf "$dataPath/phase2-dump2"
rm -rf "$dataPath/phase1-diffs"
rm -rf "$dataPath/phase2-diffs"  

# run erigon for a while to sync to the unwind point to capture the dump
timeout 40s ./build/bin/cdk-erigon \
    --datadir="$dataPath/rpc-datadir" \
    --config=./.dynamic-configs/dynamic-integration8.yaml \
    --zkevm.sync-limit=${firstStop}

# now get a dump of the datadir at this point
go run ./cmd/hack --action=dumpAll --chaindata="$dataPath/rpc-datadir/chaindata" --output="$dataPath/phase1-dump1"

# now run to the final stop block
timeout 15s ./build/bin/cdk-erigon \
    --datadir="$dataPath/rpc-datadir" \
    --config=./.dynamic-configs/dynamic-integration8.yaml \
    --zkevm.sync-limit=${stopBlock}

# now get a dump of the datadir at this point
go run ./cmd/hack --action=dumpAll --chaindata="$dataPath/rpc-datadir/chaindata" --output="$dataPath/phase2-dump1"

# now run the unwind
go run ./cmd/integration state_stages_zkevm \
    --datadir="$dataPath/rpc-datadir" \
    --config=./.dynamic-configs/dynamic-integration8.yaml \
    --chain=dynamic-integration \
    --unwind-batch-no=${unwindBatch}

# now get a dump of the datadir at this point
go run ./cmd/hack --action=dumpAll --chaindata="$dataPath/rpc-datadir/chaindata" --output="$dataPath/phase1-dump2"

# now sync again
timeout 15s ./build/bin/cdk-erigon \
    --datadir="$dataPath/rpc-datadir" \
    --config=./.dynamic-configs/dynamic-integration8.yaml \
    --zkevm.sync-limit=${stopBlock}

# dump the data again into the post folder
go run ./cmd/hack --action=dumpAll --chaindata="$dataPath/rpc-datadir/chaindata" --output="$dataPath/phase2-dump2"

mkdir -p "$dataPath/phase1-diffs/pre"
mkdir -p "$dataPath/phase1-diffs/post"
mkdir -p "$dataPath/phase2-diffs/pre"
mkdir -p "$dataPath/phase2-diffs/post"

# iterate over the files in the pre-dump folder
for file in $(ls $dataPath/phase1-dump1); do
    # get the filename
    filename=$(basename $file)

    # diff the files and if there is a difference found copy the pre and post files into the diffs folder
    if cmp -s $dataPath/phase1-dump1/$filename $dataPath/phase1-dump2/$filename; then
        echo "No difference found in $filename"
        exit(1)
    else
        cp $dataPath/phase1-dump1/$file $dataPath/phase1-diffs/pre/$filename
        cp $dataPath/phase1-dump2/$file $dataPath/phase1-diffs/post/$filename
    fi
done

# iterate over the files in the pre-dump folder
for file in $(ls $dataPath/phase2-dump1); do
    # get the filename
    filename=$(basename $file)

    # diff the files and if there is a difference found copy the pre and post files into the diffs folder
    if cmp -s $dataPath/phase2-dump1/$filename $dataPath/phase2-dump2/$filename; then
        echo "No difference found in $filename"
    else
        cp $dataPath/phase2-dump1/$file $dataPath/phase2-diffs/pre/$filename
        cp $dataPath/phase2-dump2/$file $dataPath/phase2-diffs/post/$filename
    fi
done
