# tapbot
Tap bot for discord

This is the bot for handling requests

List of available commands:
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

6. '$dump_txs <ADDRESS>' - get json file with all transactions


### how to use:
provide config in the gollowing form:

```
priv-key = "YOURKEYHERE"
pub-key = "YOURKEYHERE"
address ="ADDR"

# discord bot token
token= "TOKEN"

# 100000000000 = 0.1 SMH
transfer-amount= 300
fee= 50
# how often user can request tokens from faucet
cooldown= "10s"
# gateway for node API
server= "api-devnet208.spacemesh.io:9092"
```
then run from terminal 
  ` ./tap`
  
