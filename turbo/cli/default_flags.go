package cli

import (
	"github.com/urfave/cli/v2"

	"github.com/ledgerwatch/erigon/cmd/utils"
)

// DefaultFlags contains all flags that are used and supported by Erigon binary.
var DefaultFlags = []cli.Flag{
	&utils.DataDirFlag,
	&utils.EthashDatasetDirFlag,
	&utils.SnapshotFlag,
	&utils.ExternalConsensusFlag,
	&utils.InternalConsensusFlag,
	&utils.TxPoolDisableFlag,
	&utils.TxPoolLocalsFlag,
	&utils.TxPoolNoLocalsFlag,
	&utils.TxPoolPriceLimitFlag,
	&utils.TxPoolPriceBumpFlag,
	&utils.TxPoolBlobPriceBumpFlag,
	&utils.TxPoolAccountSlotsFlag,
	&utils.TxPoolBlobSlotsFlag,
	&utils.TxPoolTotalBlobPoolLimit,
	&utils.TxPoolGlobalSlotsFlag,
	&utils.TxPoolGlobalBaseFeeSlotsFlag,
	&utils.TxPoolAccountQueueFlag,
	&utils.TxPoolGlobalQueueFlag,
	&utils.TxPoolLifetimeFlag,
	&utils.TxPoolTraceSendersFlag,
	&utils.TxPoolCommitEveryFlag,
	&utils.TxpoolPurgeEveryFlag,
	&utils.TxpoolPurgeDistanceFlag,
	&PruneFlag,
	&PruneHistoryFlag,
	&PruneReceiptFlag,
	&PruneTxIndexFlag,
	&PruneCallTracesFlag,
	&PruneHistoryBeforeFlag,
	&PruneReceiptBeforeFlag,
	&PruneTxIndexBeforeFlag,
	&PruneCallTracesBeforeFlag,
	&BatchSizeFlag,
	&BodyCacheLimitFlag,
	&DatabaseVerbosityFlag,
	&PrivateApiAddr,
	&PrivateApiRateLimit,
	&EtlBufferSizeFlag,
	&TLSFlag,
	&TLSCertFlag,
	&TLSKeyFlag,
	&TLSCACertFlag,
	&StateStreamDisableFlag,
	&SyncLoopThrottleFlag,
	&BadBlockFlag,

	&utils.HTTPEnabledFlag,
	&utils.HTTPServerEnabledFlag,
	&utils.GraphQLEnabledFlag,
	&utils.HTTPListenAddrFlag,
	&utils.HTTPPortFlag,
	&utils.AuthRpcAddr,
	&utils.AuthRpcPort,
	&utils.JWTSecretPath,
	&utils.HttpCompressionFlag,
	&utils.HTTPCORSDomainFlag,
	&utils.HTTPVirtualHostsFlag,
	&utils.AuthRpcVirtualHostsFlag,
	&utils.HTTPApiFlag,
	&utils.WSPortFlag,
	&utils.WSEnabledFlag,
	&utils.WSListenAddrFlag,
	&utils.WSApiFlag,
	&utils.WsCompressionFlag,
	&utils.HTTPTraceFlag,
	&utils.HTTPDebugSingleFlag,
	&utils.StateCacheFlag,
	&utils.RpcBatchConcurrencyFlag,
	&utils.RpcStreamingDisableFlag,
	&utils.DBReadConcurrencyFlag,
	&utils.RpcAccessListFlag,
	&utils.RpcTraceCompatFlag,
	&utils.RpcGasCapFlag,
	&utils.RpcBatchLimit,
	&utils.RpcReturnDataLimit,
	&utils.RpcLogsMaxRange,
	&utils.AllowUnprotectedTxs,
	&utils.RpcMaxGetProofRewindBlockCount,
	&utils.RPCGlobalTxFeeCapFlag,
	&utils.TxpoolApiAddrFlag,
	&utils.TraceMaxtracesFlag,
	&HTTPReadTimeoutFlag,
	&HTTPWriteTimeoutFlag,
	&HTTPIdleTimeoutFlag,
	&AuthRpcReadTimeoutFlag,
	&AuthRpcWriteTimeoutFlag,
	&AuthRpcIdleTimeoutFlag,
	&EvmCallTimeoutFlag,
	&OverlayGetLogsFlag,
	&OverlayReplayBlockFlag,

	&utils.SnapKeepBlocksFlag,
	&utils.SnapStopFlag,
	&utils.DbPageSizeFlag,
	&utils.DbSizeLimitFlag,
	&utils.ForcePartialCommitFlag,
	&utils.TorrentPortFlag,
	&utils.TorrentMaxPeersFlag,
	&utils.TorrentConnsPerFileFlag,
	&utils.TorrentDownloadSlotsFlag,
	&utils.TorrentStaticPeersFlag,
	&utils.TorrentUploadRateFlag,
	&utils.TorrentDownloadRateFlag,
	&utils.TorrentVerbosityFlag,
	&utils.ListenPortFlag,
	&utils.P2pProtocolVersionFlag,
	&utils.P2pProtocolAllowedPorts,
	&utils.NATFlag,
	&utils.NoDiscoverFlag,
	&utils.DiscoveryV5Flag,
	&utils.NetrestrictFlag,
	&utils.NodeKeyFileFlag,
	&utils.NodeKeyHexFlag,
	&utils.DNSDiscoveryFlag,
	&utils.BootnodesFlag,
	&utils.StaticPeersFlag,
	&utils.TrustedPeersFlag,
	&utils.MaxPeersFlag,
	&utils.ChainFlag,
	&utils.DeveloperPeriodFlag,
	&utils.VMEnableDebugFlag,
	&utils.NetworkIdFlag,
	&utils.FakePoWFlag,
	&utils.GpoBlocksFlag,
	&utils.GpoPercentileFlag,
	&utils.InsecureUnlockAllowedFlag,
	&utils.IdentityFlag,
	&utils.CliqueSnapshotCheckpointIntervalFlag,
	&utils.CliqueSnapshotInmemorySnapshotsFlag,
	&utils.CliqueSnapshotInmemorySignaturesFlag,
	&utils.CliqueDataDirFlag,
	&utils.MiningEnabledFlag,
	&utils.ProposingDisableFlag,
	&utils.MinerNotifyFlag,
	&utils.MinerGasLimitFlag,
	&utils.MinerEtherbaseFlag,
	&utils.MinerExtraDataFlag,
	&utils.MinerNoVerfiyFlag,
	&utils.MinerSigningKeyFileFlag,
	&utils.MinerRecommitIntervalFlag,
	&utils.SentryAddrFlag,
	&utils.SentryLogPeerInfoFlag,
	&utils.DownloaderAddrFlag,
	&utils.DisableIPV4,
	&utils.DisableIPV6,
	&utils.NoDownloaderFlag,
	&utils.DownloaderVerifyFlag,
	&HealthCheckFlag,
	&utils.HeimdallURLFlag,
	&utils.WebSeedsFlag,
	&utils.WithoutHeimdallFlag,
	&utils.BorBlockPeriodFlag,
	&utils.BorBlockSizeFlag,
	&utils.WithHeimdallMilestones,
	&utils.WithHeimdallWaypoints,
	&utils.PolygonSyncFlag,
	&utils.EthStatsURLFlag,
	&utils.OverridePragueFlag,

	&utils.LightClientDiscoveryAddrFlag,
	&utils.LightClientDiscoveryPortFlag,
	&utils.LightClientDiscoveryTCPPortFlag,
	&utils.SentinelAddrFlag,
	&utils.SentinelPortFlag,
	&utils.YieldSizeFlag,

	&utils.L2ChainIdFlag,
	&utils.L2RpcUrlFlag,
	&utils.L2DataStreamerUrlFlag,
	&utils.L2DataStreamerMaxEntryChanFlag,
	&utils.L2DataStreamerUseTLSFlag,
	&utils.L2DataStreamerTimeout,
	&utils.L2ShortCircuitToVerifiedBatchFlag,
	&utils.L1SyncStartBlock,
	&utils.L1SyncStopBatch,
	&utils.L1ChainIdFlag,
	&utils.L1RpcUrlFlag,
	&utils.L1CacheEnabledFlag,
	&utils.L1CachePortFlag,
	&utils.AddressSequencerFlag,
	&utils.AddressAdminFlag,
	&utils.AddressRollupFlag,
	&utils.AddressZkevmFlag,
	&utils.AddressGerManagerFlag,
	&utils.L1RollupIdFlag,
	&utils.L1BlockRangeFlag,
	&utils.L1QueryDelayFlag,
	&utils.L1HighestBlockTypeFlag,
	&utils.L1MaticContractAddressFlag,
	&utils.L1FirstBlockFlag,
	&utils.L1FinalizedBlockRequirementFlag,
	&utils.L1ContractAddressCheckFlag,
	&utils.L1ContractAddressRetrieveFlag,
	&utils.RpcRateLimitsFlag,
	&utils.RpcGetBatchWitnessConcurrencyLimitFlag,
	&utils.RebuildTreeAfterFlag,
	&utils.IncrementTreeAlways,
	&utils.SmtRegenerateInMemory,
	&utils.SequencerBlockSealTime,
	&utils.SequencerEmptyBlockSealTime,
	&utils.SequencerBatchSealTime,
	&utils.SequencerBatchVerificationTimeout,
	&utils.SequencerBatchVerificationRetries,
	&utils.SequencerTimeoutOnEmptyTxPool,
	&utils.SequencerHaltOnBatchNumber,
	&utils.SequencerResequence,
	&utils.SequencerResequenceStrict,
	&utils.SequencerResequenceReuseL1InfoIndex,
	&utils.ExecutorUrls,
	&utils.ExecutorStrictMode,
	&utils.ExecutorRequestTimeout,
	&utils.ExecutorEnabled,
	&utils.DatastreamNewBlockTimeout,
	&utils.WitnessMemdbSize,
	&utils.WitnessUnwindLimit,
	&utils.ExecutorMaxConcurrentRequests,
	&utils.Limbo,
	&utils.AllowFreeTransactions,
	&utils.AllowPreEIP155Transactions,
	&utils.EffectiveGasPriceForEthTransfer,
	&utils.EffectiveGasPriceForErc20Transfer,
	&utils.EffectiveGasPriceForContractInvocation,
	&utils.EffectiveGasPriceForContractDeployment,
	&utils.DefaultGasPrice,
	&utils.MaxGasPrice,
	&utils.GasPriceFactor,
	&utils.DataStreamHost,
	&utils.DataStreamPort,
	&utils.DataStreamWriteTimeout,
	&utils.DataStreamInactivityTimeout,
	&utils.DataStreamInactivityCheckInterval,
	&utils.WitnessFullFlag,
	&utils.SyncLimit,
	&utils.SyncLimitVerifiedEnabled,
	&utils.SyncLimitUnverifiedCount,
	&utils.ExecutorPayloadOutput,
	&utils.DebugTimers,
	&utils.DebugNoSync,
	&utils.DebugLimit,
	&utils.DebugStep,
	&utils.DebugStepAfter,
	&utils.OtsSearchMaxCapFlag,
	&utils.PanicOnReorg,
	&utils.ShadowSequencer,
	&utils.ZKGenesisConfigPathFlag,

	&utils.SilkwormExecutionFlag,
	&utils.SilkwormRpcDaemonFlag,
	&utils.SilkwormSentryFlag,
	&utils.SilkwormVerbosityFlag,
	&utils.SilkwormNumContextsFlag,
	&utils.SilkwormRpcLogEnabledFlag,
	&utils.SilkwormRpcLogMaxFileSizeFlag,
	&utils.SilkwormRpcLogMaxFilesFlag,
	&utils.SilkwormRpcLogDumpResponseFlag,
	&utils.SilkwormRpcNumWorkersFlag,
	&utils.SilkwormRpcJsonCompatibilityFlag,

	&utils.BeaconAPIFlag,
	&utils.BeaconApiAddrFlag,
	&utils.BeaconApiAllowMethodsFlag,
	&utils.BeaconApiAllowOriginsFlag,
	&utils.BeaconApiAllowCredentialsFlag,
	&utils.BeaconApiPortFlag,
	&utils.BeaconApiReadTimeoutFlag,
	&utils.BeaconApiWriteTimeoutFlag,
	&utils.BeaconApiProtocolFlag,
	&utils.BeaconApiIdleTimeoutFlag,

	&utils.CaplinBackfillingFlag,
	&utils.CaplinBlobBackfillingFlag,
	&utils.CaplinDisableBlobPruningFlag,
	&utils.CaplinArchiveFlag,

	&utils.TrustedSetupFile,
	&utils.RPCSlowFlag,

	&utils.TxPoolGossipDisableFlag,
	&SyncLoopBlockLimitFlag,
	&SyncLoopBreakAfterFlag,
	&SyncLoopPruneLimitFlag,
	&utils.PoolManagerUrl,
	&utils.TxPoolRejectSmartContractDeployments,
	&utils.DisableVirtualCounters,
	&utils.DAUrl,
	&utils.VirtualCountersSmtReduction,
	&utils.BadBatches,
	&utils.IgnoreBadBatchesCheck,
	&utils.InitialBatchCfgFile,

	&utils.ACLPrintHistory,
	&utils.InfoTreeUpdateInterval,
	&utils.SealBatchImmediatelyOnOverflow,
	&utils.MockWitnessGeneration,
	&utils.WitnessCacheEnable,
	&utils.WitnessCachePurge,
	&utils.WitnessCacheBatchAheadOffset,
	&utils.WitnessCacheBatchBehindOffset,
	&utils.WitnessContractInclusion,
	&utils.GasPriceCheckFrequency,
	&utils.GasPriceHistoryCount,
	&utils.RejectLowGasPriceTransactions,
	&utils.RejectLowGasPriceTolerance,
	&utils.BadTxAllowance,
	&utils.BadTxStoreValue,
	&utils.BadTxPurge,
}
