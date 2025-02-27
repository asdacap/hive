package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/holiman/uint256"
	blsu "github.com/protolambda/bls12-381-util"
	"github.com/protolambda/ztyp/view"

	"github.com/ethereum/hive/hivesim"
	"github.com/ethereum/hive/simulators/eth2/testnet/setup"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/configs"
)

var depositAddress common.Eth1Address

func init() {
	_ = depositAddress.UnmarshalText([]byte("0x4242424242424242424242424242424242424242"))
}

// PreparedTestnet has all the options for starting nodes, ready to build the network.
type PreparedTestnet struct {
	// Consensus chain configuration
	spec *common.Spec

	// Execution chain configuration and genesis info
	eth1Genesis *setup.Eth1Genesis
	// Consensus genesis state
	eth2Genesis common.BeaconState
	// Secret keys of validators, to fabricate extra signed test messages with during testnet/
	// E.g. to test a slashable offence that would not otherwise happen.
	keys *[]blsu.SecretKey

	// Configuration to apply to every node of the given type
	executionOpts hivesim.StartOption
	validatorOpts hivesim.StartOption
	beaconOpts    hivesim.StartOption

	// A tranche is a group of validator keys to run on 1 node
	keyTranches [][]*setup.KeyDetails
}

// Build all artifacts require to start a testnet.
func prepareTestnet(t *hivesim.T, env *testEnv, config *config) *PreparedTestnet {
	eth1GenesisTime := common.Timestamp(time.Now().Unix())
	eth2GenesisTime := eth1GenesisTime + 30

	// Generate genesis for execution clients
	eth1Genesis := setup.BuildEth1Genesis(config.TerminalTotalDifficulty, uint64(eth1GenesisTime), config.Eth1Consensus == Clique)
	eth1ConfigOpt := eth1Genesis.ToParams(depositAddress)
	eth1Bundle, err := setup.Eth1Bundle(eth1Genesis.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	execNodeOpts := hivesim.Params{"HIVE_LOGLEVEL": os.Getenv("HIVE_LOGLEVEL")}
	executionOpts := hivesim.Bundle(eth1ConfigOpt, eth1Bundle, execNodeOpts)

	// Generate beacon spec
	//
	// TODO: specify build-target based on preset, to run clients in mainnet or minimal mode.
	// copy the default mainnet config, and make some minimal modifications for testnet usage
	specCpy := *configs.Mainnet
	spec := &specCpy
	spec.Config.DEPOSIT_CONTRACT_ADDRESS = depositAddress
	spec.Config.DEPOSIT_CHAIN_ID = eth1Genesis.Genesis.Config.ChainID.Uint64()
	spec.Config.DEPOSIT_NETWORK_ID = eth1Genesis.NetworkID
	spec.Config.ETH1_FOLLOW_DISTANCE = 1

	spec.Config.ALTAIR_FORK_EPOCH = common.Epoch(config.AltairForkEpoch)
	spec.Config.BELLATRIX_FORK_EPOCH = common.Epoch(config.MergeForkEpoch)
	spec.Config.MIN_GENESIS_ACTIVE_VALIDATOR_COUNT = config.ValidatorCount
	spec.Config.SECONDS_PER_SLOT = common.Timestamp(config.SlotTime)
	tdd, _ := uint256.FromBig(config.TerminalTotalDifficulty)
	spec.Config.TERMINAL_TOTAL_DIFFICULTY = view.Uint256View(*tdd)

	// Generate keys opts for validators
	keyTranches := setup.KeyTranches(env.Keys, uint64(len(config.Nodes)))

	consensusConfigOpts, err := setup.ConsensusConfigsBundle(spec, eth1Genesis.Genesis, config.ValidatorCount)
	if err != nil {
		t.Fatal(err)
	}

	// prepare genesis beacon state, with all the validators in it.
	state, err := setup.BuildBeaconState(spec, eth1Genesis.Genesis, eth2GenesisTime, env.Keys)
	if err != nil {
		t.Fatal(err)
	}

	// Write info so that the genesis state can be generated by the client
	stateOpt, err := setup.StateBundle(state)
	if err != nil {
		t.Fatal(err)
	}

	// Define additional start options for beacon chain
	commonOpts := hivesim.Params{
		"HIVE_ETH2_BN_API_PORT":                     fmt.Sprintf("%d", PortBeaconAPI),
		"HIVE_ETH2_BN_GRPC_PORT":                    fmt.Sprintf("%d", PortBeaconGRPC),
		"HIVE_ETH2_METRICS_PORT":                    fmt.Sprintf("%d", PortMetrics),
		"HIVE_ETH2_CONFIG_DEPOSIT_CONTRACT_ADDRESS": depositAddress.String(),
	}
	beaconOpts := hivesim.Bundle(
		commonOpts,
		hivesim.Params{
			"HIVE_CHECK_LIVE_PORT":        fmt.Sprintf("%d", PortBeaconAPI),
			"HIVE_ETH2_MERGE_ENABLED":     "1",
			"HIVE_ETH2_ETH1_GENESIS_TIME": fmt.Sprintf("%d", eth1Genesis.Genesis.Timestamp),
			"HIVE_ETH2_GENESIS_FORK":      config.activeFork(),
		},
		stateOpt,
		consensusConfigOpts,
	)

	validatorOpts := hivesim.Bundle(
		commonOpts,
		hivesim.Params{
			"HIVE_CHECK_LIVE_PORT": "0",
		},
		consensusConfigOpts,
	)

	return &PreparedTestnet{
		spec:          spec,
		eth1Genesis:   eth1Genesis,
		eth2Genesis:   state,
		keys:          env.Secrets,
		executionOpts: executionOpts,
		beaconOpts:    beaconOpts,
		validatorOpts: validatorOpts,
		keyTranches:   keyTranches,
	}
}

func (p *PreparedTestnet) createTestnet(t *hivesim.T) *Testnet {
	genesisTime, _ := p.eth2Genesis.GenesisTime()
	genesisValidatorsRoot, _ := p.eth2Genesis.GenesisValidatorsRoot()
	return &Testnet{
		t:                     t,
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
		spec:                  p.spec,
		eth1Genesis:           p.eth1Genesis,
	}
}

func (p *PreparedTestnet) startEth1Node(testnet *Testnet, eth1Def *hivesim.ClientDefinition, consensus ConsensusType) {
	testnet.t.Logf("Starting eth1 node: %s (%s)", eth1Def.Name, eth1Def.Version)

	opts := []hivesim.StartOption{p.executionOpts}
	if len(testnet.eth1) == 0 {
		// we only make the first eth1 node a miner
		if consensus == Ethash {
			opts = append(opts, hivesim.Params{"HIVE_MINER": "1212121212121212121212121212121212121212"})
		} else if consensus == Clique {
			opts = append(opts, hivesim.Params{
				"HIVE_CLIQUE_PRIVATEKEY": "9c647b8b7c4e7c3490668fb6c11473619db80c93704c70893d3813af4090c39c",
				"HIVE_MINER":             "658bdf435d810c91414ec09147daa6db62406379",
			})
		}
	} else {
		bootnode, err := testnet.eth1[0].EnodeURL()
		if err != nil {
			testnet.t.Fatalf("failed to get eth1 bootnode URL: %v", err)
		}

		// Make the client connect to the first eth1 node, as a bootnode for the eth1 net
		opts = append(opts, hivesim.Params{"HIVE_BOOTNODE": bootnode})
	}
	en := &Eth1Node{testnet.t.StartClient(eth1Def.Name, opts...)}
	testnet.eth1 = append(testnet.eth1, en)
}

func (p *PreparedTestnet) startBeaconNode(testnet *Testnet, beaconDef *hivesim.ClientDefinition, eth1Endpoints []int) {
	testnet.t.Logf("Starting beacon node: %s (%s)", beaconDef.Name, beaconDef.Version)

	opts := []hivesim.StartOption{p.beaconOpts}
	// Hook up beacon node to (maybe multiple) eth1 nodes
	for _, index := range eth1Endpoints {
		if index < 0 || index >= len(testnet.eth1) {
			testnet.t.Fatalf("only have %d eth1 nodes, cannot find index %d for BN", len(testnet.eth1), index)
		}
	}

	var addrs []string
	var engineAddrs []string
	for _, index := range eth1Endpoints {
		eth1Node := testnet.eth1[index]
		userRPC, err := eth1Node.UserRPCAddress()
		if err != nil {
			testnet.t.Fatalf("eth1 node used for beacon without available RPC: %v", err)
		}
		addrs = append(addrs, userRPC)
		engineRPC, err := eth1Node.EngineRPCAddress()
		if err != nil {
			testnet.t.Fatalf("eth1 node used for beacon without available RPC: %v", err)
		}
		engineAddrs = append(engineAddrs, engineRPC)
	}
	opts = append(opts, hivesim.Params{
		"HIVE_ETH2_ETH1_RPC_ADDRS":        strings.Join(addrs, ","),
		"HIVE_ETH2_ETH1_ENGINE_RPC_ADDRS": strings.Join(engineAddrs, ","),
	})

	if len(testnet.beacons) > 0 {
		bootnodeENR, err := testnet.beacons[0].ENR()
		if err != nil {
			testnet.t.Fatalf("failed to get ENR as bootnode for beacon node: %v", err)
		}
		opts = append(opts, hivesim.Params{"HIVE_ETH2_BOOTNODE_ENRS": bootnodeENR})
	}

	// TODO
	//if p.configName != "mainnet" && hasBuildTarget(beaconDef, p.configName) {
	//	opts = append(opts, hivesim.WithBuildTarget(p.configName))
	//}
	bn := NewBeaconNode(testnet.t.StartClient(beaconDef.Name, opts...))
	testnet.beacons = append(testnet.beacons, bn)
}

func (p *PreparedTestnet) startValidatorClient(testnet *Testnet, validatorDef *hivesim.ClientDefinition, bnIndex int, keyIndex int) {
	testnet.t.Logf("Starting validator client: %s (%s)", validatorDef.Name, validatorDef.Version)

	if bnIndex >= len(testnet.beacons) {
		testnet.t.Fatalf("only have %d beacon nodes, cannot find index %d for VC", len(testnet.beacons), bnIndex)
	}
	bn := testnet.beacons[bnIndex]
	// Hook up validator to beacon node
	bnAPIOpt := hivesim.Params{
		"HIVE_ETH2_BN_API_IP": bn.IP.String(),
	}
	if keyIndex >= len(p.keyTranches) {
		testnet.t.Fatalf("only have %d key tranches, cannot find index %d for VC", len(p.keyTranches), keyIndex)
	}
	keysOpt := setup.KeysBundle(p.keyTranches[keyIndex])
	opts := []hivesim.StartOption{p.validatorOpts, keysOpt, bnAPIOpt}
	// TODO
	//if p.configName != "mainnet" && hasBuildTarget(validatorDef, p.configName) {
	//	opts = append(opts, hivesim.WithBuildTarget(p.configName))
	//}
	vc := &ValidatorClient{testnet.t.StartClient(validatorDef.Name, opts...), p.keyTranches[keyIndex]}
	testnet.validators = append(testnet.validators, vc)
}
