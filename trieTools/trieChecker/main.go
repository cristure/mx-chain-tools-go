package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ElrondNetwork/elrond-go-core/core"
	"github.com/ElrondNetwork/elrond-go-core/core/pubkeyConverter"
	logger "github.com/ElrondNetwork/elrond-go-logger"
	"github.com/ElrondNetwork/elrond-go/common"
	"github.com/ElrondNetwork/elrond-go/state"
	"github.com/ElrondNetwork/elrond-tools-go/trieTools/trieToolsCommon"
	"github.com/urfave/cli"
)

const (
	logFilePrefix  = "trie-checker"
	rootHashLength = 32
	addressLength  = 32
)

func main() {
	app := cli.NewApp()
	app.Name = "Trie checker CLI app"
	app.Usage = "This is the entry point for the tool that checks the trie DB"
	app.Flags = getFlags()
	app.Authors = []cli.Author{
		{
			Name:  "The Elrond Team",
			Email: "contact@elrond.com",
		},
	}

	app.Action = func(c *cli.Context) error {
		return startProcess(c)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
		return
	}

	log.Info("finished processing trie")
}

func startProcess(c *cli.Context) error {
	flagsConfig := getFlagsConfig(c)

	_, errLogger := trieToolsCommon.AttachFileLogger(log, logFilePrefix, flagsConfig)
	if errLogger != nil {
		return errLogger
	}

	log.Info("sanity checks...")

	err := logger.SetLogLevel(flagsConfig.LogLevel)
	if err != nil {
		return err
	}

	rootHash, err := hex.DecodeString(flagsConfig.HexRootHash)
	if err != nil {
		return fmt.Errorf("%w when decoding the provided hex root hash", err)
	}
	if len(rootHash) != rootHashLength {
		return fmt.Errorf("wrong root hash length: expected %d, got %d", rootHashLength, len(rootHash))
	}

	maxDBValue, err := trieToolsCommon.GetMaxDBValue(filepath.Join(flagsConfig.WorkingDir, flagsConfig.DbDir), log)
	if err != nil {
		return err
	}

	log.Info("starting processing trie", "pid", os.Getpid())

	return checkTrie(flagsConfig, rootHash, maxDBValue)
}

func checkTrie(flags trieToolsCommon.ContextFlagsConfig, mainRootHash []byte, maxDBValue int) error {
	addressConverter, err := pubkeyConverter.NewBech32PubkeyConverter(addressLength, log)
	if err != nil {
		return err
	}

	tr, err := trieToolsCommon.GetTrie(flags, maxDBValue)
	if err != nil {
		return err
	}

	defer func() {
		errNotCritical := tr.Close()
		log.LogIfError(errNotCritical)
	}()

	ch := make(chan core.KeyValueHolder, common.TrieLeavesChannelDefaultCapacity)
	err = tr.GetAllLeavesOnChannel(ch, context.Background(), mainRootHash)
	if err != nil {
		return err
	}

	numAccountsOnMainTrie := 0
	numCodeNodes := 0
	dataTriesRootHashes := make(map[string][]byte)
	numDataTriesLeaves := 0
	for kv := range ch {
		numAccountsOnMainTrie++

		userAccount := &state.UserAccountData{}
		errUnmarshal := trieToolsCommon.Marshaller.Unmarshal(userAccount, kv.Value())
		if errUnmarshal != nil {
			// probably a code node
			numCodeNodes++
			continue
		}
		if len(userAccount.RootHash) == 0 {
			continue
		}

		address := addressConverter.Encode(kv.Key())
		dataTriesRootHashes[address] = userAccount.RootHash
	}

	log.Info("parsed main trie",
		"num accounts", numAccountsOnMainTrie,
		"num code nodes", numCodeNodes,
		"num data tries", len(dataTriesRootHashes))

	// TODO add error signaling in the trie implementation

	if len(dataTriesRootHashes) == 0 {
		return nil
	}

	for address, dataRootHash := range dataTriesRootHashes {
		log.Debug("iterating data trie", "address", address, "data trie root hash", dataRootHash)

		chDataTrie := make(chan core.KeyValueHolder, common.TrieLeavesChannelDefaultCapacity)
		errGetAllLeaves := tr.GetAllLeavesOnChannel(chDataTrie, context.Background(), dataRootHash)
		if errGetAllLeaves != nil {
			return errGetAllLeaves
		}

		for range chDataTrie {
			numDataTriesLeaves++
		}
	}

	log.Info("parsed all tries",
		"num accounts", numAccountsOnMainTrie,
		"num code nodes", numCodeNodes,
		"num data tries", len(dataTriesRootHashes),
		"num data tries leaves", numDataTriesLeaves)

	return nil
}
