package query

import (
	"github.com/cosmos/cosmos-sdk/codec"
	vestingexported "github.com/cosmos/cosmos-sdk/x/auth/vesting/exported"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/supply"

	"github.com/ovrclk/cosmos-supply-summary/sdkutil"
	"github.com/ovrclk/cosmos-supply-summary/x/supply/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/exported"
	abci "github.com/tendermint/tendermint/abci/types"
)

// NewQuerier creates and returns a new supply querier instance
func NewQuerier(cdc *codec.Codec, accKeeper types.AccountKeeper, supKeeper types.SupplyKeeper,
	stKeeper types.StakingKeeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) (res []byte, err error) {
		switch path[0] {
		case circulatingPath:
			return queryCirculatingSupply(ctx, cdc, accKeeper, supKeeper, stKeeper)
		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unknown query for endpoint: %s", path[0])
		}
	}
}

func queryCirculatingSupply(ctx sdk.Context, cdc *codec.Codec, accKeeper types.AccountKeeper,
	supKeeper types.SupplyKeeper, stKeeper types.StakingKeeper) (res []byte, err error) {
	var supplyData Supply
	supplyData.Total = supKeeper.GetSupply(ctx).GetTotal()

	delegationsMap := make(map[string]sdk.Coins)
	stKeeper.IterateAllDelegations(ctx, func(delegation staking.Delegation) bool {
		// Converting delegated shares to sdk.Coin
		delegated := sdk.NewCoin(stKeeper.BondDenom(ctx), delegation.Shares.TruncateInt())
		delegationsMap[delegation.DelegatorAddress.String()] = delegationsMap[delegation.DelegatorAddress.String()].Add(delegated)
		return false
	})
	accKeeper.IterateAccounts(ctx, func(account exported.Account) bool {
		if ma, ok := account.(*supply.ModuleAccount); ok {
			switch ma.Name {
			case staking.NotBondedPoolName, staking.BondedPoolName:
				return false
			}
		}
		delegatedTokens := delegationsMap[account.GetAddress().String()]
		va, ok := account.(vestingexported.VestingAccount)
		if !ok {
			supplyData.Available.Bonded = supplyData.Available.Bonded.Add(delegatedTokens...)
			supplyData.Available.Unbonded = supplyData.Available.Unbonded.Add(account.GetCoins()...)
		} else {
			supplyData.Vesting.Bonded = supplyData.Vesting.Bonded.Add(va.GetDelegatedVesting()...)
			supplyData.Vesting.Unbonded = supplyData.Vesting.Unbonded.Add(va.GetCoins()...).Sub(va.SpendableCoins(ctx.BlockTime()))
			supplyData.Available.Bonded = supplyData.Available.Bonded.Add(delegatedTokens...).Sub(va.GetDelegatedVesting())
			supplyData.Available.Unbonded = supplyData.Available.Unbonded.Add(va.SpendableCoins(ctx.BlockTime())...)
		}
		return false
	})

	supplyData.Circulating = supplyData.Total.Sub(supplyData.Vesting.Unbonded).Sub(supplyData.Vesting.Bonded)
	return sdkutil.RenderQueryResponse(cdc, supplyData)
}
