package ethereum

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/renproject/multichain/api/account"
	"github.com/renproject/multichain/api/address"
	"github.com/renproject/multichain/api/contract"
	"github.com/renproject/pack"
)

const (
	// DefaultClientRPCURL is the RPC URL used by default, to interact with the
	// ethereum node.
	DefaultClientRPCURL = "http://127.0.0.1:8545/"
)

// Client holds the underlying RPC client instance.
type Client struct {
	ethClient *ethclient.Client
}

// NewClient creates and returns a new JSON-RPC client to the Ethereum node
func NewClient(rpcURL string) (*Client, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("dialing url: %v", rpcURL))
	}
	return &Client{
		client,
	}, nil
}

// LatestBlock returns the block number at the current chain head.
func (client *Client) LatestBlock(ctx context.Context) (pack.U64, error) {
	header, err := client.ethClient.HeaderByNumber(ctx, nil)
	if err != nil {
		return pack.NewU64(0), fmt.Errorf("fetching header: %v", err)
	}
	return pack.NewU64(header.Number.Uint64()), nil
}

// Tx returns the transaction uniquely identified by the given transaction
// hash. It also returns the number of confirmations for the transaction.
func (client *Client) Tx(ctx context.Context, txID pack.Bytes) (account.Tx, pack.U64, error) {
	tx, pending, err := client.ethClient.TransactionByHash(ctx, common.BytesToHash(txID))
	if err != nil {
		return nil, pack.NewU64(0), fmt.Errorf(fmt.Sprintf("fetching tx by hash '%v': %v", txID, err))
	}
	chainID, err := client.ethClient.ChainID(ctx)
	if err != nil {
		return nil, pack.NewU64(0), fmt.Errorf("fetching chain ID: %v", err)
	}

	// If the transaction is still pending, use default EIP-155 signer.
	pendingTx := Tx{
		ethTx:  tx,
		signer: types.NewEIP155Signer(chainID),
	}
	if pending {
		return &pendingTx, 0, nil
	}

	receipt, err := client.ethClient.TransactionReceipt(ctx, common.BytesToHash(txID))
	if err != nil {
		return nil, pack.NewU64(0), fmt.Errorf("fetching recipt for tx %v : %v", txID, err)
	}

	// if no receipt, tx has 0 confirmations
	if receipt == nil {
		return &pendingTx, 0, nil
	}

	// reverted tx
	if receipt.Status == 0 {
		return nil, pack.NewU64(0), fmt.Errorf("tx %v reverted, reciept status 0", txID)
	}

	// tx confirmed
	confirmedTx := Tx{
		tx,
		types.LatestSignerForChainID(chainID),
	}

	// select signer for chain
	switch chainID.Uint64() {
	case 0:
		confirmedTx = Tx{
			tx,
			types.MakeSigner(params.YoloV3ChainConfig, receipt.BlockNumber),
		}
	case 1:
		confirmedTx = Tx{
			tx,
			types.MakeSigner(params.MainnetChainConfig, receipt.BlockNumber),
		}
	case 3:
		confirmedTx = Tx{
			tx,
			types.MakeSigner(params.RopstenChainConfig, receipt.BlockNumber),
		}
	case 4:
		confirmedTx = Tx{
			tx,
			types.MakeSigner(params.RinkebyChainConfig, receipt.BlockNumber),
		}
	case 5:
		confirmedTx = Tx{
			tx,
			types.MakeSigner(params.GoerliChainConfig, receipt.BlockNumber),
		}
	}

	header, err := client.ethClient.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, pack.NewU64(0), fmt.Errorf("fetching header : %v", err)
	}

	return &confirmedTx, pack.NewU64(header.Number.Uint64() - receipt.BlockNumber.Uint64()), nil
}

// SubmitTx to the underlying blockchain network.
func (client *Client) SubmitTx(ctx context.Context, tx account.Tx) error {
	switch tx := tx.(type) {
	case *Tx:
		err := client.ethClient.SendTransaction(ctx, tx.ethTx)
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("sending transaction '%v': %v", tx.Hash(), err))
		}
		return nil
	default:
		return fmt.Errorf("expected type %T, got type %T", new(Tx), tx)
	}
}

// AccountNonce returns the current nonce of the account. This is the nonce to
// be used while building a new transaction.
func (client *Client) AccountNonce(ctx context.Context, addr address.Address) (pack.U256, error) {
	targetAddr, err := NewAddressFromHex(string(pack.String(addr)))
	if err != nil {
		return pack.U256{}, fmt.Errorf("bad to address '%v': %v", addr, err)
	}
	nonce, err := client.ethClient.NonceAt(ctx, common.Address(targetAddr), nil)
	if err != nil {
		return pack.U256{}, fmt.Errorf("failed to get nonce for '%v': %v", addr, err)
	}

	return pack.NewU256FromU64(pack.NewU64(nonce)), nil
}

// AccountBalance returns the account balancee for a given address.
func (client *Client) AccountBalance(ctx context.Context, addr address.Address) (pack.U256, error) {
	targetAddr, err := NewAddressFromHex(string(pack.String(addr)))
	if err != nil {
		return pack.U256{}, fmt.Errorf("bad to address '%v': %v", addr, err)
	}
	balance, err := client.ethClient.BalanceAt(ctx, common.Address(targetAddr), nil)
	if err != nil {
		return pack.U256{}, fmt.Errorf("failed to get balance for '%v': %v", addr, err)
	}

	return pack.NewU256FromInt(balance), nil
}

// CallContract implements the multichain Contract API.
func (client *Client) CallContract(ctx context.Context, program address.Address, calldata contract.CallData) (pack.Bytes, error) {
	targetAddr, err := NewAddressFromHex(string(pack.String(program)))
	if err != nil {
		return nil, fmt.Errorf("bad to address '%v': %v", program, err)
	}
	addr := common.Address(targetAddr)

	callMsg := ethereum.CallMsg{
		To:   &addr,
		Data: calldata,
	}
	return client.ethClient.CallContract(ctx, callMsg, nil)
}
