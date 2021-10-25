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

	spllited := strings.Split(m.Content, " ")
	println("got new message ", m.Content)

	if handler, has := b.handlers[spllited[0]]; has {
		out, err := handler(spllited)
		if err != nil {
			println(err.Error())
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
		return "", fmt.Errorf("wrong address format")
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
		return "", fmt.Errorf("wrong address format")
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
		return "", fmt.Errorf("wrong address format")
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

/*
@client.event
async def on_message(message):
    session = aiohttp.ClientSession()
    message_timestamp = time.time()
    requester = message.author
    usr1 = client.get_user(id=USER_ID_NOTIFY)

    # Do not listen to your own messages
    if message.author == client.user:
        return

    if message.content.startswith('$balance'):
        address = str(message.content).replace("$balance", "").replace(" ", "").lower()
        if str(address[:2]) == "0x" and len(address) == 42:
            balance = await spacemesh_api.get_balance(session, address)
            if "error" in str(balance).lower():
                await message.channel.send(f'{message.author.mention} {str(balance)}')
            else:
                await message.channel.send(f'{message.author.mention}, {str(balance)} smidge ({int(balance) / decimal:.3f} SMH)')

    if message.content.startswith('$help'):
        await message.channel.send(help_msg)

    if message.content.startswith('$dump_txs'):
        address = str(message.content).replace("$dump_txs", "").replace(" ", "").lower()
        if str(address[:2]).lower() == "0x" and len(address) == 42:
            await spacemesh_api.dump_all_transactions(session, address.lower())
            await requester.send(file=discord.File(f"{address[:15]}.json"))

    # Show node synchronization settings
    if message.content.startswith('$faucet_status'):
        print(requester.name, "status request")
        try:
            faucet_balance = await spacemesh_api.get_balance(session, ADDRESS)
            status = await spacemesh_api.get_node_status(session)
            if "synced" in status and "ERROR" not in str(faucet_balance):
                status = f'```' \
                         f'Balance: {int(faucet_balance) / decimal} SMH\n' \
                         f'Peers:   {status["peers"]}\n' \
                         f'Synced:  {status["synced"]}\n' \
                         f'Layers:  {status["currentLayer"]}\\{status["syncedLayer"]}\n```'
            await message.channel.send(status)

        except Exception as statusErr:
            print(statusErr)

    if message.content.startswith('$faucet_address') or message.content.startswith('$tap_address') and message.channel.name in LISTENING_CHANNELS:
        try:
            await message.channel.send(f"Faucet address is: {ADDRESS}")
        except:
            print("Can't send message $faucet_address")

    if message.content.startswith('$tx_info') and message.channel.name in LISTENING_CHANNELS:
        try:
            hash_id = str(message.content).replace("$tx_info", "").replace(" ", "")
            if len(hash_id) == 64 or len(hash_id) == 66:
                tr_info = await spacemesh_api.get_transaction_info(session, hash_id)
                if "amount" and "fee" in str(tr_info):
                    tr_info = f'```' \
                              f'From:       0x{str(tr_info["sender"]["address"])}\n' \
                              f'To:         0x{str(tr_info["receiver"]["address"])}\n' \
                              f'Amount:     {float(int(tr_info["amount"]) / decimal)} SMH\n' \
                              f'Fee:        {int(tr_info["fee"])}\n' \
                              f'STATUS:     {tr_info["status"]}```'
                await message.channel.send(tr_info)
            else:
                await message.channel.send(f'Incorrect len hash id {hash_id}')

        except Exception as tx_infoErr:
            print(tx_infoErr)

    if str(message.content[:2]).lower() == "0x" and len(message.content) == 42 and message.channel.name in LISTENING_CHANNELS:
        channel = message.channel
        requester_address = str(message.content).lower()

        if requester.id in ACTIVE_REQUESTS:
            check_time = ACTIVE_REQUESTS[requester.id]["next_request"]
            if check_time > message_timestamp:
                please_wait_text = f'{requester.mention}, You can request coins no more than once every 3 hours.' \
                                   f'The next attempt is possible after ' \
                                   f'{round((check_time - message_timestamp) / 60, 2)} minutes'
                await channel.send(please_wait_text)
                return

            else:
                del ACTIVE_REQUESTS[requester.id]

        if requester.id not in ACTIVE_REQUESTS and requester_address not in ACTIVE_REQUESTS:
            ACTIVE_REQUESTS[requester.id] = {
                "address": requester_address,
                "requester": requester,
                "next_request": message_timestamp + REQUEST_COLDOWN}
            print(ACTIVE_REQUESTS)

            faucet_balance = int(await spacemesh_api.get_balance(session, ADDRESS))
            if faucet_balance > (FAUCET_AMOUNT + FAUCET_FEE):
                transaction = await spacemesh_api.send_transaction(session,
                                                                   frm=ADDRESS,
                                                                   to=requester_address,
                                                                   amount=FAUCET_AMOUNT,
                                                                   gas_price=FAUCET_FEE,
                                                                   private_key=PRIVATE_KEY)
                logger.info(f'Transaction result:\n{transaction}')
                if transaction["value"] == "ok":
                    await message.add_reaction(emoji=APPROVE_EMOJI)
                    confirm_time = await spacemesh_api.tx_subscription(session, transaction["id"])
                    if confirm_time == "removed":
                        await message.add_reaction(emoji=REJECT_EMOJI)
                        await message.channel.send(f'{requester.mention}, {transaction["id"]} was fail to send. '
                                                   f'You can do another request')
                        # remove the restriction on the request for coins, since the transaction was not completed
                        del ACTIVE_REQUESTS[requester.id]

                    elif confirm_time != "timeout":
                        await message.add_reaction(emoji=CONFIRMED_EMOJI)

                    elif confirm_time == "timeout":
                        await message.channel.send(f'{requester.mention}, Transaction confirmation took more than '
                                                   f'{CONFIRM_TIMEOUT_MIN} minutes. '
                                                   f'Check status manually: `$tx_info {str(transaction["id"])}`')
                        await usr1.send(f'Transaction confirmation took more than {CONFIRM_TIMEOUT_MIN} minutes: '
                                        f'{transaction["id"]}')

                    # await message.channel.send(f'TX_ID: {transaction["id"]} | Confirmation time: {confirm_time}')
                    await save_transaction_statistics(f'{transaction["id"]};{confirm_time}')
                    await session.close()

            elif faucet_balance < (FAUCET_AMOUNT + FAUCET_FEE):
                logger.error(f'Insufficient funds: {faucet_balance}')
                await message.add_reaction(emoji=REJECT_EMOJI)
                await message.channel.send(f'@yaelh#5158,\n'
                                           f'Insufficient funds: {faucet_balance}. '
                                           f'It is necessary to replenish the faucet address: `{ADDRESS}`')

*/
