# #!/bin/bash

# # Define the folders
# # folder1="/mnt/Working/Gateway/projects/GatewayData/hermez-mainnet-shadowfork-dump-5-backward"
# # folder2="/mnt/Working/Gateway/projects/GatewayData/hermez-mainnet-shadowfork-dump-5-forward"
# folder1="/mnt/Working/Gateway/projects/GatewayData/hermez-dynamic-integration8-dump-backward"
# folder2="/mnt/Working/Gateway/projects/GatewayData/hermez-dynamic-integration8-dump-forward"

# # Check if both folders exist
# if [ ! -d "$folder1" ] || [ ! -d "$folder2" ]; then
#   echo "One or both of the folders do not exist."
#   exit 1
# fi

# # Iterate over the files in the first folder
# for file1 in "$folder1"/*; do
#   # Get the filename without the path
#   filename=$(basename "$file1")
  
#   # Construct the corresponding file path in the second folder
#   file2="$folder2/$filename"
  
#   # Check if the corresponding file exists in the second folder
#   if [ -f "$file2" ]; then
#     # Calculate the SHA-256 checksums
#     sha1=$(sha256sum "$file1" | awk '{print $1}')
#     sha2=$(sha256sum "$file2" | awk '{print $1}')
    
#     # Compare the checksums
#     if [ "$sha1" != "$sha2" ]; then
#       echo "Files $filename differ."
#     fi
#   else
#     echo "File $filename does not exist in $folder2."
#   fi
# done


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

dataPath="/mnt/Working/Gateway/projects/GatewayData/witness-debug"
firstStop=11204
stopBlock=11315
unwindBatch=70
# firstStop=2
# stopBlock=192
# stopBlock=372
# unwindBatch=1

rm -rf "$dataPath/rpc-datadir"
rm -rf "$dataPath/phase1-dump1"
rm -rf "$dataPath/phase1-dump2"
rm -rf "$dataPath/phase2-dump1"
rm -rf "$dataPath/phase2-dump2"
rm -rf "$dataPath/phase1-diffs"
rm -rf "$dataPath/phase2-diffs"  

# cp -r "$dataPath/../hermez-cardona-with-logs" "$dataPath/rpc-datadir"

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
