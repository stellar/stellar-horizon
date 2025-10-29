package contractevents

import (
	"encoding/hex"
	"fmt"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
	"math"
	"math/big"
)

// GenerateEvent is a utility function to be used by testing frameworks in order
// to generate Stellar Asset Contract events.
//
// To provide a generic interface, there are more arguments than apply to the
// type, but you should only expect the relevant ones to be set (for example,
// transfer events have no admin, so it will be ignored). This means you can
// always pass your set of testing parameters, modify the type, and get the
// event filled out with the details you expect.
func GenerateEvent(
	type_ EventType,
	from, to, admin string,
	asset xdr.Asset,
	amount *big.Int,
	passphrase string,
	muxedInfo *xdr.Memo,
) xdr.ContractEvent {
	var topics []xdr.ScVal
	var data xdr.ScVal
	if muxedInfo != nil {
		data = makeV4MapData(amount, *muxedInfo)
	} else {
		data = makeBigAmount(amount)

	}

	switch type_ {
	case EventTypeTransfer:
		topics = []xdr.ScVal{
			makeSymbol("transfer"),
			makeAddress(from),
			makeAddress(to),
			makeAsset(asset),
		}

	case EventTypeMint:
		topics = []xdr.ScVal{
			makeSymbol("mint"),
			makeAddress(admin),
			makeAddress(to),
			makeAsset(asset),
		}

	case EventTypeClawback:
		topics = []xdr.ScVal{
			makeSymbol("clawback"),
			makeAddress(admin),
			makeAddress(from),
			makeAsset(asset),
		}

	case EventTypeBurn:
		topics = []xdr.ScVal{
			makeSymbol("burn"),
			makeAddress(from),
			makeAsset(asset),
		}

	default:
		panic(fmt.Errorf("event type %v unsupported", type_))
	}

	rawContractId, err := asset.ContractID(passphrase)
	if err != nil {
		panic(err)
	}
	contractId := xdr.ContractId(rawContractId)

	event := xdr.ContractEvent{
		Type:       xdr.ContractEventTypeContract,
		ContractId: &contractId,
		Body: xdr.ContractEventBody{
			V: 0,
			V0: &xdr.ContractEventV0{
				Topics: xdr.ScVec(topics),
				Data:   data,
			},
		},
	}

	return event
}

func contractIdToHash(contractId string) *xdr.ContractId {
	idBytes := [32]byte{}
	rawBytes, err := hex.DecodeString(contractId)
	if err != nil {
		panic(fmt.Errorf("invalid contract id (%s): %v", contractId, err))
	}
	if copy(idBytes[:], rawBytes[:]) != 32 {
		panic("couldn't copy 32 bytes to contract hash")
	}

	hash := xdr.ContractId(idBytes)
	return &hash
}

func makeSymbol(sym string) xdr.ScVal {
	symbol := xdr.ScSymbol(sym)
	return xdr.ScVal{
		Type: xdr.ScValTypeScvSymbol,
		Sym:  &symbol,
	}
}

func makeBigAmount(amount *big.Int) xdr.ScVal {
	if amount.BitLen() > 128 {
		panic(fmt.Errorf("amount is too large: %d bits (max 128)", amount.BitLen()))
	}

	keepLower := big.NewInt(0).SetUint64(math.MaxUint64)
	hi := new(big.Int).Rsh(amount, 64)
	lo := amount.And(amount, keepLower)

	return xdr.ScVal{
		Type: xdr.ScValTypeScvI128,
		I128: &xdr.Int128Parts{
			Lo: xdr.Uint64(lo.Uint64()),
			Hi: xdr.Int64(hi.Int64()),
		},
	}
}

func makeAddress(address string) xdr.ScVal {
	scAddress := xdr.ScAddress{}

	switch address[0] {
	case 'C':
		scAddress.Type = xdr.ScAddressTypeScAddressTypeContract
		contractHash := strkey.MustDecode(strkey.VersionByteContract, address)
		scAddress.ContractId = contractIdToHash(hex.EncodeToString(contractHash))
	case 'G':
		scAddress.Type = xdr.ScAddressTypeScAddressTypeAccount
		scAddress.AccountId = xdr.MustAddressPtr(address)
	default:
		panic(fmt.Errorf("unsupported address: %s", address))
	}

	return xdr.ScVal{
		Type:    xdr.ScValTypeScvAddress,
		Address: &scAddress,
	}
}

func makeAsset(asset xdr.Asset) xdr.ScVal {
	assetScStr := xdr.ScString(asset.StringCanonical())
	return xdr.ScVal{
		Type: xdr.ScValTypeScvString,
		Str:  &assetScStr,
	}
}

func makeV4MapData(amount *big.Int, memo xdr.Memo) xdr.ScVal {
	mapEntries := xdr.ScMap{}

	// Add amount entry
	amountEntry := xdr.ScMapEntry{
		Key: xdr.ScVal{
			Type: xdr.ScValTypeScvSymbol,
			Sym:  &[]xdr.ScSymbol{"amount"}[0],
		},
		Val: makeBigAmount(amount),
	}
	mapEntries = append(mapEntries, amountEntry)

	// Add to_muxed_id entry based on memo type
	var muxedIdVal xdr.ScVal
	switch memo.Type {
	case xdr.MemoTypeMemoId:
		id := memo.Id
		val := *id
		muxedIdVal = xdr.ScVal{
			Type: xdr.ScValTypeScvU64,
			U64:  &val,
		}
	case xdr.MemoTypeMemoText:
		str := memo.Text
		val := xdr.ScString(*str)
		muxedIdVal = xdr.ScVal{
			Type: xdr.ScValTypeScvString,
			Str:  &val,
		}
	case xdr.MemoTypeMemoHash:
		bytes := xdr.ScBytes(memo.Hash[:])
		muxedIdVal = xdr.ScVal{
			Type:  xdr.ScValTypeScvBytes,
			Bytes: &bytes,
		}
	default:
		panic(fmt.Errorf("unsupported memo type: %v", memo.Type))
	}

	muxedIdEntry := xdr.ScMapEntry{
		Key: xdr.ScVal{
			Type: xdr.ScValTypeScvSymbol,
			Sym:  &[]xdr.ScSymbol{"to_muxed_id"}[0],
		},
		Val: muxedIdVal,
	}
	mapEntries = append(mapEntries, muxedIdEntry)
	mapPtr := &mapEntries

	// Need to use double pointer for Map field
	return xdr.ScVal{
		Type: xdr.ScValTypeScvMap,
		Map:  &mapPtr,
	}
}
