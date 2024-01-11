package stagedsync

import (
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/turbo/stages/bodydownload"
)

type DownloaderGlue interface {
	SpawnHeaderDownloadStage([]func() error, *stages.StageState, stages.Unwinder) error
	SpawnBodyDownloadStage(string, string, *stages.StageState, stages.Unwinder, *bodydownload.PrefetchedBlocks) (bool, error)
}
