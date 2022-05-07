package bot

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/bwmarrin/discordgo"
	xdr "github.com/nullstyle/go-xdr/xdr3"
	apitypes "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/spacemeshos/ed25519"
	gosmtypes "github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/common/util"
	"strings"
	"time"
)

type Client interface {
	NodeStatus() (*apitypes.NodeStatus, error)
	AccountState(address gosmtypes.Address) (*apitypes.Account, error)
	Transfer(recipient gosmtypes.Address, nonce, amount, gasPrice, gasLimit uint64, key ed25519.PrivateKey) (*apitypes.TransactionState, error)
	TransactionState(txId []byte, includeTx bool) (*apitypes.TransactionState, *apitypes.Transaction, error)
	GetMeshTransactions(address gosmtypes.Address, offset uint32, maxResults uint32) ([]*apitypes.MeshTransaction, uint32, error)
}

const DefaultTxAmount = 1000
const TranserBackoffSeconds = 300

const helpText = `**List of available commands:**
1. Request coins through the tap - send your address
*You can request coins no more than once every three hours*

Transaction status explanation:
💸 - mean bot send transaction to your address, but the transaction has not yet been confirmed
✅ - transaction was successfully confirmed
🚫 - the transaction was not confirmed for some reason. You need to make another request
*Bot track transaction status only for 15 minutes*
*Average transaction confirmation time 10-13 minutes*

2. '$faucet_status\' - displays the current status of the node where faucet is running

3. '$faucet_address' or '$tap_address' - show tap address

4. '$tx_info <TX_ID>' - show transaction information for a specific transaction ID
(sender, receiver, fee, amount, status)

5. '$balance <ADDRESS>' - show address balance

6. '$dump_txs <ADDRESS>' - get json file with all transactions"""`

type botBackend struct {
	backend  Client
	key      ed25519.PrivateKey
	public   gosmtypes.Address
	backoff  map[string]time.Time
	handlers map[string]func(cmd []string) (string, error)
	cfg      BaseConfig
}

func NewBot(backend Client, publicKey gosmtypes.Address, key ed25519.PrivateKey, cfg BaseConfig) *botBackend {
	b := &botBackend{
		backend:  backend,
		key:      key,
		public:   publicKey,
		backoff:  make(map[string]time.Time),
		handlers: make(map[string]func(cmd []string) (string, error)),
		cfg:      cfg,
	}
	b.handlers = map[string]func(cmd []string) (string, error){
		balance:      b.getBalance,
		help:         b.getHelp,
		faucetStatus: b.getFaucetStatus,
		faucetAddr:   b.getFaucetAddress,
		txInfo:       b.getTxInfo,
		dumpTxs:      b.getDumpTx}

	return b
}

var transactionStateDisStringsMap = map[int32]string{
	0: "Unspecified state",
	1: "Rejected",
	2: "Insufficient funds",
	3: "Conflicting",
	4: "Submitted to the network",
	5: "On the mesh but not yet processed",
	6: "Processed",
}

const (
	balance      = "$balance"
	help         = "$help"
	dumpTxs      = "$dump_txs"
	faucetStatus = "$faucet_status"
	faucetAddr   = "$faucet_addr"
	txInfo       = "$tx_info"
)

func (b *botBackend) OnMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	ch, err := s.Channel(m.ChannelID)
	if err != nil {
		println("no channel ", err)
		return
	}
	if ch.Name != "tap" {
		return
	}
	spllited := strings.Split(m.Content, " ")
	println("got new message ", m.Content)

	if handler, has := b.handlers[spllited[0]]; has {
		out, err := handler(spllited)
		if err != nil {
			println(err.Error())
			_, err = s.ChannelMessageSend(m.ChannelID, err.Error())
			return
		}
		_, err = s.ChannelMessageSend(m.ChannelID, out)
		if err != nil {
			println(err.Error())
		}
	} else {
		if strings.HasPrefix(strings.ToLower(spllited[0]), "0x") {
			out, err := b.transferFunds(spllited)
			if err != nil {
				println(err.Error())
				_, _ = s.ChannelMessageSend(m.ChannelID, err.Error())
				return
			}
			_, err = s.ChannelMessageSend(m.ChannelID, out)
			if err != nil {
				println(err.Error())
			}

		}
	}

}

func (b *botBackend) getBalance(cmd []string) (string, error) {
	if len(cmd) < 2 {
		return "", fmt.Errorf("account name not provided")
	}
	address := gosmtypes.BytesToAddress(util.FromHex(cmd[1]))
	if address.Big().Uint64() == 0 {
		return "", fmt.Errorf("invalid address, enter a valid wallet address (40 hex chars with 0x prefix)")
	}
	state, err := b.backend.AccountState(address)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("account %v balance %v", address.String(), state.GetStateCurrent().Balance.Value), nil
}

func (b *botBackend) getHelp(cmd []string) (string, error) {
	return helpText, nil
}

func (b *botBackend) getDumpTx(cmd []string) (string, error) {
	if len(cmd) < 2 {
		return "", fmt.Errorf("account name not provided")
	}
	address := gosmtypes.BytesToAddress(util.FromHex(cmd[1]))
	if address.Big().Uint64() == 0 {
		return "", fmt.Errorf("invalid address, enter a valid wallet address (40 hex chars with 0x prefix)")
	}
	txs, _, err := b.backend.GetMeshTransactions(address, 0, 100)
	if err != nil {
		return "", err
	}
	str := ""
	for _, tx := range txs {
		str += getTxStr(tx)
	}
	return str, nil
}

func getTxStr(tranasction *apitypes.MeshTransaction) string {
	tx := tranasction.Transaction
	ct := tx.GetCoinTransfer()
	msg := fmt.Sprintf("tx info:\nfrom: %v\nto: %v\namount: %v\nfee: %v\nlayer:%v\n",
		gosmtypes.BytesToAddress(tx.Sender.Address).String(),
		gosmtypes.BytesToAddress(ct.Receiver.Address).String(),
		tx.Amount,
		tx.GasOffered,
		tranasction.LayerId.Number)
	return msg
}

func (b *botBackend) getFaucetStatus(cmd []string) (string, error) {
	address := b.public
	if address.Big().Uint64() == 0 {
		return "", fmt.Errorf("invalid address, enter a valid wallet address (40 hex chars with 0x prefix)")
	}
	state, err := b.backend.AccountState(address)
	if err != nil {
		return "", err
	}

	status, err := b.backend.NodeStatus()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Balance: %v\n Synced: %v\n Peers: %v\n Layer :%v", state.StateProjected.Balance, status.IsSynced, status.ConnectedPeers, status.TopLayer), nil
}

func (b *botBackend) getFundAmount() uint64 {
	return b.cfg.TransferAmount
}

func (b *botBackend) getFaucetAddr() gosmtypes.Address {
	return b.public
}

func (b *botBackend) getFaucetAddress(cmd []string) (string, error) {
	return b.public.String(), nil
}

func (b *botBackend) getFaucetPrivateKey() ed25519.PrivateKey {
	return b.key
}

func (b *botBackend) getTxInfo(cmd []string) (string, error) {
	addr := cmd[1]
	bts := util.FromHex(addr)
	state, tx, err := b.backend.TransactionState(bts, true)
	if err != nil {
		return "", err
	}
	ct := tx.GetCoinTransfer()
	msg := fmt.Sprintf("tx info: from: %v\nto %v\namount %v\nfee %v\nstatus %v", gosmtypes.BytesToAddress(tx.Sender.Address).String(), gosmtypes.BytesToAddress(ct.Receiver.Address).String(), tx.Amount, tx.GasOffered, state.State.String())
	return msg, nil
}

func (b *botBackend) transferFunds(cmd []string) (string, error) {
	if err := b.canSubmitTransactions(); err != nil {
		return "", err
	}

	destAddressStr := cmd[0]
	if len(destAddressStr) < 42 {
		return "", fmt.Errorf("address is invalid, please enter a 42 digit address with 0x prefix")
	}
	destAddress, err := gosmtypes.StringToAddress(destAddressStr)
	if err != nil {
		return "", err
	}

	amount := uint64(b.cfg.TransferAmount) //todo: default amount
	gas := uint64(50)

	account, err := b.backend.AccountState(b.getFaucetAddr())
	if err != nil {
		println("err reading faucet status")
	}

	fmt.Println("New transaction summary:")
	fmt.Println("To:    ", destAddress.String())
	fmt.Println("Nonce: ", account.StateProjected.Counter)

	state, err := b.backend.AccountState(b.getFaucetAddr())
	if err != nil {
		return "", err
	}

	if state.StateProjected.Balance.Value < b.getFundAmount()+gas {
		return "", fmt.Errorf("insufficient funds")
	}

	if ts, ok := b.backoff[destAddress.String()]; ok {
		if time.Now().Before(ts) {
			return "", fmt.Errorf("account %v requested funds too soon", destAddress.String())
		}
	}

	txState, err := b.backend.Transfer(destAddress, account.StateProjected.Counter, amount, gas, 100, b.getFaucetPrivateKey())
	if err != nil {
		return "", fmt.Errorf("🚫 tx rejected by node, %v", err)
	}

	txStateDispString := transactionStateDisStringsMap[int32(txState.State.Number())]
	fmt.Println("Transaction submitted.")
	fmt.Println(fmt.Sprintf("Transaction id: 0x%v", hex.EncodeToString(txState.Id.Id)))
	fmt.Println("Transaction state:", txStateDispString)

	if txState.State <= apitypes.TransactionState_TRANSACTION_STATE_CONFLICTING {
		return "", fmt.Errorf("🚫 tx rejected by node, %v", txStateDispString)
	}

	b.backoff[destAddress.String()] = time.Now().Add(b.cfg.RequestCoolDown)

	return fmt.Sprintf("💸  transferred funds to %v\n txID: %v", destAddress.String(), "0x"+Bytes2Hex(txState.Id.Id)), nil
}

// canSubmitTransactions returns true if the node is accepting transactions.
// todo: this should move to a method in the transactions service.
func (b *botBackend) canSubmitTransactions() error {
	status, err := b.backend.NodeStatus()
	if err != nil {
		return fmt.Errorf("node not available %v", err)
	}

	// for now, we allow to submit txs if the node is synced && status.TopLayer.Number > minVerifiedLayer
	if status.IsSynced != true {
		return fmt.Errorf("node not synced")
	}

	return nil
}

func interfaceToBytes(i interface{}) ([]byte, error) {
	var w bytes.Buffer
	if _, err := xdr.Marshal(&w, &i); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}
