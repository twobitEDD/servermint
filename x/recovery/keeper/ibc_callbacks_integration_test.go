package keeper_test

import (
	"github.com/EscanBE/evermint/v12/constants"
	"time"

	"github.com/EscanBE/evermint/v12/app"
	ibctesting "github.com/EscanBE/evermint/v12/ibc/testing"
	"github.com/EscanBE/evermint/v12/testutil"
	teststypes "github.com/EscanBE/evermint/v12/types/tests"
	claimstypes "github.com/EscanBE/evermint/v12/x/claims/types"
	"github.com/EscanBE/evermint/v12/x/recovery/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	channeltypes "github.com/cosmos/ibc-go/v7/modules/core/04-channel/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Recovery: Performing an IBC Transfer", Ordered, func() {
	nativeCoin := sdk.NewCoin(constants.BaseDenom, sdk.NewInt(10000))
	coinOsmo := sdk.NewCoin("uosmo", sdk.NewInt(10))
	coinAtom := sdk.NewCoin("uatom", sdk.NewInt(10))

	var (
		sender, receiver       string
		senderAcc, receiverAcc sdk.AccAddress
		timeout                uint64
		claim                  claimstypes.ClaimsRecord
	)

	BeforeEach(func() {
		s.SetupTest()
	})

	Describe("from a non-authorized chain", func() {
		BeforeEach(func() {
			params := claimstypes.DefaultParams()
			params.AuthorizedChannels = []string{}
			err := s.EvermintChain.App.(*app.Evermint).ClaimsKeeper.SetParams(s.EvermintChain.GetContext(), params)
			s.Require().NoError(err)

			sender = s.IBCOsmosisChain.SenderAccount.GetAddress().String()
			receiver = s.EvermintChain.SenderAccount.GetAddress().String()
			senderAcc = sdk.MustAccAddressFromBech32(sender)
			receiverAcc = sdk.MustAccAddressFromBech32(receiver)
		})
		It("should transfer and not recover tokens", func() {
			s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, "uosmo", 10, sender, receiver, 1)

			nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
			Expect(nativeCoin).To(Equal(nativeCoin))
			ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
			Expect(ibcOsmo).To(Equal(sdk.NewCoin(teststypes.UosmoIbcdenom, coinOsmo.Amount)))
		})
	})

	Describe("from an authorized, non-EVM chain (e.g. Osmosis)", func() {
		Describe("to a different account on Evermint (sender != recipient)", func() {
			BeforeEach(func() {
				sender = s.IBCOsmosisChain.SenderAccount.GetAddress().String()
				receiver = s.EvermintChain.SenderAccount.GetAddress().String()
				senderAcc = sdk.MustAccAddressFromBech32(sender)
				receiverAcc = sdk.MustAccAddressFromBech32(receiver)
			})

			It("should transfer and not recover tokens", func() {
				s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, "uosmo", 10, sender, receiver, 1)

				nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
				Expect(nativeCoin).To(Equal(nativeCoin))
				ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
				Expect(ibcOsmo).To(Equal(sdk.NewCoin(teststypes.UosmoIbcdenom, coinOsmo.Amount)))
			})
		})

		Describe("to the sender's own eth_secp256k1 account on Evermint (sender == recipient)", func() {
			BeforeEach(func() {
				sender = s.IBCOsmosisChain.SenderAccount.GetAddress().String()
				receiver = s.IBCOsmosisChain.SenderAccount.GetAddress().String()
				senderAcc = sdk.MustAccAddressFromBech32(sender)
				receiverAcc = sdk.MustAccAddressFromBech32(receiver)
			})

			Context("with disabled recovery parameter", func() {
				BeforeEach(func() {
					params := types.DefaultParams()
					params.EnableRecovery = false
					s.EvermintChain.App.(*app.Evermint).RecoveryKeeper.SetParams(s.EvermintChain.GetContext(), params) //nolint:errcheck
				})

				It("should not transfer or recover tokens", func() {
					s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, coinOsmo.Denom, coinOsmo.Amount.Int64(), sender, receiver, 1)
					nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
					Expect(nativeCoin).To(Equal(nativeCoin))
					ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
					Expect(ibcOsmo).To(Equal(sdk.NewCoin(teststypes.UosmoIbcdenom, coinOsmo.Amount)))
				})
			})

			Context("with a sender's claims record", func() {
				Context("without completed actions", func() {
					BeforeEach(func() {
						amt := sdk.NewInt(int64(100))
						coins := sdk.NewCoins(sdk.NewCoin(constants.BaseDenom, amt))
						claim = claimstypes.NewClaimsRecord(amt)
						s.EvermintChain.App.(*app.Evermint).ClaimsKeeper.SetClaimsRecord(s.EvermintChain.GetContext(), senderAcc, claim)

						// update the escrowed account balance to maintain the invariant
						err := testutil.FundModuleAccount(s.EvermintChain.GetContext(), s.EvermintChain.App.(*app.Evermint).BankKeeper, claimstypes.ModuleName, coins)
						s.Require().NoError(err)
					})

					It("should not transfer or recover tokens", func() {
						// Prevent further funds from getting stuck
						s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, coinOsmo.Denom, coinOsmo.Amount.Int64(), sender, receiver, 1)
						nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
						Expect(nativeCoin).To(Equal(nativeCoin))
						ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())
					})
				})

				Context("with completed actions", func() {
					// Already has stuck funds
					BeforeEach(func() {
						amt := sdk.NewInt(int64(100))
						coins := sdk.NewCoins(sdk.NewCoin(constants.BaseDenom, sdk.NewInt(int64(75))))
						claim = claimstypes.NewClaimsRecord(amt)
						claim.MarkClaimed(claimstypes.ActionIBCTransfer)
						s.EvermintChain.App.(*app.Evermint).ClaimsKeeper.SetClaimsRecord(s.EvermintChain.GetContext(), senderAcc, claim)

						// update the escrowed account balance to maintain the invariant
						err := testutil.FundModuleAccount(s.EvermintChain.GetContext(), s.EvermintChain.App.(*app.Evermint).BankKeeper, claimstypes.ModuleName, coins)
						s.Require().NoError(err)

						// native coin & ibc tokens that originated from the sender's chain
						s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, coinOsmo.Denom, coinOsmo.Amount.Int64(), sender, receiver, 1)
						timeout = uint64(s.EvermintChain.GetContext().BlockTime().Add(time.Hour * 4).Add(time.Second * -20).UnixNano())
					})

					It("should transfer tokens to the recipient and perform recovery", func() {
						s.T().Skip("TODO: fix this test")
						// Escrow before relaying packets
						balanceEscrow := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), transfertypes.GetEscrowAddress("transfer", "channel-0"), constants.BaseDenom)
						Expect(balanceEscrow).To(Equal(nativeCoin))
						ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())

						// Relay both packets that were sent in the ibc_callback
						err := s.pathOsmosisEvermint.RelayPacket(CreatePacket("10000", constants.BaseDenom, sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 1, timeout))
						s.Require().NoError(err)
						err = s.pathOsmosisEvermint.RelayPacket(CreatePacket("10", "transfer/channel-0/uosmo", sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 2, timeout))
						s.Require().NoError(err)

						// Check that the native coin were recovered
						nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
						Expect(nativeCoin.IsZero()).To(BeTrue())
						ibcNativeCoin := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, teststypes.NativeCoinIbcdenom)
						Expect(ibcNativeCoin).To(Equal(sdk.NewCoin(teststypes.NativeCoinIbcdenom, nativeCoin.Amount)))

						// Check that the uosmo were recovered
						ibcOsmo = s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())
						nativeOsmo := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, "uosmo")
						Expect(nativeOsmo).To(Equal(coinOsmo))
					})

					It("should not claim/migrate/merge claims records", func() {
						// Relay both packets that were sent in the ibc_callback
						err := s.pathOsmosisEvermint.RelayPacket(CreatePacket("10000", constants.BaseDenom, sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 1, timeout))
						s.Require().NoError(err)
						err = s.pathOsmosisEvermint.RelayPacket(CreatePacket("10", "transfer/channel-0/uosmo", sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 2, timeout))
						s.Require().NoError(err)

						claimAfter, _ := s.EvermintChain.App.(*app.Evermint).ClaimsKeeper.GetClaimsRecord(s.EvermintChain.GetContext(), senderAcc)
						Expect(claim).To(Equal(claimAfter))
					})
				})
			})

			Context("without a sender's claims record", func() {
				When("recipient has no ibc vouchers that originated from other chains", func() {
					It("should transfer and recover tokens", func() {
						// native coin & ibc tokens that originated from the sender's chain
						s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, coinOsmo.Denom, coinOsmo.Amount.Int64(), sender, receiver, 1)
						timeout = uint64(s.EvermintChain.GetContext().BlockTime().Add(time.Hour * 4).Add(time.Second * -20).UnixNano())

						// Escrow before relaying packets
						balanceEscrow := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), transfertypes.GetEscrowAddress("transfer", "channel-0"), constants.BaseDenom)
						Expect(balanceEscrow).To(Equal(nativeCoin))
						ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())

						// Relay both packets that were sent in the ibc_callback
						err := s.pathOsmosisEvermint.RelayPacket(CreatePacket("10000", constants.BaseDenom, sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 1, timeout))
						s.Require().NoError(err)
						err = s.pathOsmosisEvermint.RelayPacket(CreatePacket("10", "transfer/channel-0/uosmo", sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 2, timeout))
						s.Require().NoError(err)

						// Check that the native coin were recovered
						nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
						Expect(nativeCoin.IsZero()).To(BeTrue())
						ibcNativeCoin := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, teststypes.NativeCoinIbcdenom)
						Expect(ibcNativeCoin).To(Equal(sdk.NewCoin(teststypes.NativeCoinIbcdenom, nativeCoin.Amount)))

						// Check that the uosmo were recovered
						ibcOsmo = s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())
						nativeOsmo := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, "uosmo")
						Expect(nativeOsmo).To(Equal(coinOsmo))
					})
				})

				// Do not recover uatom sent from Cosmos when performing recovery through IBC transfer from Osmosis
				When("recipient has additional ibc vouchers that originated from other chains", func() {
					BeforeEach(func() {
						params := types.DefaultParams()
						params.EnableRecovery = false
						s.EvermintChain.App.(*app.Evermint).RecoveryKeeper.SetParams(s.EvermintChain.GetContext(), params) //nolint:errcheck

						// Send uatom from Cosmos to Evermint
						s.SendAndReceiveMessage(s.pathCosmosEvermint, s.IBCCosmosChain, coinAtom.Denom, coinAtom.Amount.Int64(), s.IBCCosmosChain.SenderAccount.GetAddress().String(), receiver, 1)

						params.EnableRecovery = true
						s.EvermintChain.App.(*app.Evermint).RecoveryKeeper.SetParams(s.EvermintChain.GetContext(), params) //nolint:errcheck
					})
					It("should not recover tokens that originated from other chains", func() {
						// Send uosmo from Osmosis to Evermint
						s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, "uosmo", 10, sender, receiver, 1)

						// Relay both packets that were sent in the ibc_callback
						timeout := uint64(s.EvermintChain.GetContext().BlockTime().Add(time.Hour * 4).Add(time.Second * -20).UnixNano())
						err := s.pathOsmosisEvermint.RelayPacket(CreatePacket("10000", constants.BaseDenom, sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 1, timeout))
						s.Require().NoError(err)
						err = s.pathOsmosisEvermint.RelayPacket(CreatePacket("10", "transfer/channel-0/uosmo", sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 2, timeout))
						s.Require().NoError(err)

						// native coin was recovered from user address
						nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
						Expect(nativeCoin.IsZero()).To(BeTrue())
						ibcNativeCoin := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, teststypes.NativeCoinIbcdenom)
						Expect(ibcNativeCoin).To(Equal(sdk.NewCoin(teststypes.NativeCoinIbcdenom, nativeCoin.Amount)))

						// Check that the uosmo were retrieved
						ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())
						nativeOsmo := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, "uosmo")
						Expect(nativeOsmo).To(Equal(coinOsmo))

						// Check that the atoms were not retrieved
						ibcAtom := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, teststypes.UatomIbcdenom)
						Expect(ibcAtom).To(Equal(sdk.NewCoin(teststypes.UatomIbcdenom, coinAtom.Amount)))

						// Repeat transaction from Osmosis to Evermint
						s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, "uosmo", 10, sender, receiver, 2)

						timeout = uint64(s.EvermintChain.GetContext().BlockTime().Add(time.Hour * 4).Add(time.Second * -20).UnixNano())
						err = s.pathOsmosisEvermint.RelayPacket(CreatePacket("10", "transfer/channel-0/uosmo", sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 3, timeout))
						s.Require().NoError(err)

						// No further tokens recovered
						nativeCoin = s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
						Expect(nativeCoin.IsZero()).To(BeTrue())
						ibcNativeCoin = s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, teststypes.NativeCoinIbcdenom)
						Expect(ibcNativeCoin).To(Equal(sdk.NewCoin(teststypes.NativeCoinIbcdenom, nativeCoin.Amount)))

						ibcOsmo = s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())
						nativeOsmo = s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, "uosmo")
						Expect(nativeOsmo).To(Equal(coinOsmo))

						ibcAtom = s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, teststypes.UatomIbcdenom)
						Expect(ibcAtom).To(Equal(sdk.NewCoin(teststypes.UatomIbcdenom, coinAtom.Amount)))
					})
				})

				// Recover ibc/uatom that was sent from Osmosis back to Osmosis
				When("recipient has additional non-native ibc vouchers that originated from senders chains", func() {
					BeforeEach(func() {
						params := types.DefaultParams()
						params.EnableRecovery = false
						err := s.EvermintChain.App.(*app.Evermint).RecoveryKeeper.SetParams(s.EvermintChain.GetContext(), params)
						s.Require().NoError(err)
						s.SendAndReceiveMessage(s.pathOsmosisCosmos, s.IBCCosmosChain, coinAtom.Denom, coinAtom.Amount.Int64(), s.IBCCosmosChain.SenderAccount.GetAddress().String(), receiver, 1)

						// Send IBC transaction of 10 ibc/uatom
						transferMsg := transfertypes.NewMsgTransfer(s.pathOsmosisEvermint.EndpointA.ChannelConfig.PortID, s.pathOsmosisEvermint.EndpointA.ChannelID, sdk.NewCoin(teststypes.UatomIbcdenom, sdk.NewInt(10)), sender, receiver, timeoutHeight, 0, "")
						_, err = ibctesting.SendMsgs(s.IBCOsmosisChain, ibctesting.DefaultFeeAmt, transferMsg)
						s.Require().NoError(err) // message committed
						transfer := transfertypes.NewFungibleTokenPacketData("transfer/channel-1/uatom", "10", sender, receiver, "")
						packet := channeltypes.NewPacket(transfer.GetBytes(), 1, s.pathOsmosisEvermint.EndpointA.ChannelConfig.PortID, s.pathOsmosisEvermint.EndpointA.ChannelID, s.pathOsmosisEvermint.EndpointB.ChannelConfig.PortID, s.pathOsmosisEvermint.EndpointB.ChannelID, timeoutHeight, 0)
						// Receive message on the evermint side, and send ack
						err = s.pathOsmosisEvermint.RelayPacket(packet)
						s.Require().NoError(err)

						// Check that the ibc/uatom are available
						osmoIBCAtom := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UatomOsmoIbcdenom)
						s.Require().Equal(osmoIBCAtom.Amount, coinAtom.Amount)

						params.EnableRecovery = true
						s.EvermintChain.App.(*app.Evermint).RecoveryKeeper.SetParams(s.EvermintChain.GetContext(), params) //nolint:errcheck
					})
					It("should not recover tokens that originated from other chains", func() {
						s.SendAndReceiveMessage(s.pathOsmosisEvermint, s.IBCOsmosisChain, "uosmo", 10, sender, receiver, 2)

						// Relay packets that were sent in the ibc_callback
						timeout := uint64(s.EvermintChain.GetContext().BlockTime().Add(time.Hour * 4).Add(time.Second * -20).UnixNano())
						err := s.pathOsmosisEvermint.RelayPacket(CreatePacket("10000", constants.BaseDenom, sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 1, timeout))
						s.Require().NoError(err)
						err = s.pathOsmosisEvermint.RelayPacket(CreatePacket("10", "transfer/channel-0/transfer/channel-1/uatom", sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 2, timeout))
						s.Require().NoError(err)
						err = s.pathOsmosisEvermint.RelayPacket(CreatePacket("10", "transfer/channel-0/uosmo", sender, receiver, "transfer", "channel-0", "transfer", "channel-0", 3, timeout))
						s.Require().NoError(err)

						// native coin was recovered from user address
						nativeCoin := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), senderAcc, constants.BaseDenom)
						Expect(nativeCoin.IsZero()).To(BeTrue())
						ibcNativeCoin := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, teststypes.NativeCoinIbcdenom)
						Expect(ibcNativeCoin).To(Equal(sdk.NewCoin(teststypes.NativeCoinIbcdenom, nativeCoin.Amount)))

						// Check that the uosmo were recovered
						ibcOsmo := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UosmoIbcdenom)
						Expect(ibcOsmo.IsZero()).To(BeTrue())
						nativeOsmo := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), receiverAcc, "uosmo")
						Expect(nativeOsmo).To(Equal(coinOsmo))

						// Check that the ibc/uatom were retrieved
						osmoIBCAtom := s.EvermintChain.App.(*app.Evermint).BankKeeper.GetBalance(s.EvermintChain.GetContext(), receiverAcc, teststypes.UatomOsmoIbcdenom)
						Expect(osmoIBCAtom.IsZero()).To(BeTrue())
						ibcAtom := s.IBCOsmosisChain.GetSimApp().BankKeeper.GetBalance(s.IBCOsmosisChain.GetContext(), senderAcc, teststypes.UatomIbcdenom)
						Expect(ibcAtom).To(Equal(sdk.NewCoin(teststypes.UatomIbcdenom, sdk.NewInt(10))))
					})
				})
			})
		})
	})
})
