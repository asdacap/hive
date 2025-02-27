# Engine API Test Suite using Consensus Layer Mocking

The Engine API suite runs a set of tests where the execution client is started in Proof of Work mode with a certain TTD (Terminal Total Difficulty). When the TTD is reached, the client switches to produce blocks using the PoS consensus mechanism.

The PoS consensus relies on the Engine API, which receives commands from the consensus clients. This suite mimics a consensus client using a mocking tool to test specific scenarios and combinations of Engine API calls.

Clients with support for the merge are required to run this suite:
 - merge-go-ethereum
 - merge-nethermind


## Architecture

The tests by default are started by running a single execution client, and a single instance of the CL Mocker.

Each test case has a default Terminal Total Difficulty of 0, unless specified otherwise.

The secondary client can be spawned and synced if the test case requires so.

Given the following example,

    hive --client=clientA,clientB --sim=ethereum/engine

tests will have the following execution flow: 

for each testCase:
   - clientA:
      - starts (PoW or PoS, depending on TTD)
      - CL Mocker:
         - starts
         - testCaseN:
            - starts
            - (optional) clientA/clientB:
               - starts
               - stops
            - verifications complete
            - stops
         - stops
      - stops
   - clientB:
      - (Same steps repeated)
      - ...

## Engine API Test Cases

General positive and negative test cases based on the description in https://github.com/ethereum/execution-apis/blob/main/src/engine/specification.md

### Engine API Negative Test Cases:
- Invalid Terminal Block in ForkchoiceUpdated:  
Client should reject ForkchoiceUpdated directives if the referenced HeadBlockHash does not meet the TTD requirement.

- Invalid GetPayload Under PoW:  
Client must reject GetPayload directives under PoW.

- Invalid Terminal Block in NewPayload:  
Client must reject NewPayload directives if the referenced ParentHash does not meet the TTD requirement.

- Inconsistent ForkchoiceState:
Send an inconsistent ForkchoiceState with a known payload that belongs to a side chain as head, safe or finalized:
Having `A: Genesis <- P1 <- P2 <- P3`, `B: Genesis <- P1' <- P2' <- P3'`, 
send `fcU(Head: P3', Safe: P2, Finalized: P1)`, `fcU(Head: P3, Safe: P2', Finalized: P1)`, and `fcU(Head: P3, Safe: P2, Finalized: P1')`

- Unknown HeadBlockHash:  
Perform a forkchoiceUpdated call with an unknown (random) HeadBlockHash, the client should initiate the syncing process.

- Unknown SafeBlockHash:  
Perform a forkchoiceUpdated call with an unknown (random) SafeBlockHash, the client should throw an error since the hash is not an ancestor to the HeadBlockHash.

- Unknown FinalizedBlockHash:  
Perform a forkchoiceUpdated call with an unknown (random) FinalizedBlockHash, the client should throw an error.

- Invalid Payload Attributes:
Perform a forkchoiceUpdated call with valid forkchoice but invalid payload attributes.
Expected outcome is that the forkchoiceUpdate proceeds, but the call returns an error.

- Pre-TTD Block Hash:  
Perform a forkchoiceUpdated call using a block hash part of the canonical chain that precedes the block where the TTD occurred. (Behavior is undefined for this edge case and not verified, but should not produce unrecoverable error)

### Eth RPC Status on ForkchoiceUpdated Events:
- `latest` Block after NewPayload:  
Verify the Block returned by the Eth RPC after a new payload is executed. Eth RPC should still return previous block.

- `latest` Block after New HeadBlock:  
Verify the Block returned by the Eth RPC after a new HeadBlockHash is set using forkchoiceUpdated. Eth RPC should return new block.

- `safe` Block after New SafeBlockHash:  
Verify the Block returned by the Eth RPC using the `safe` keyword after a new `SafeBlockHash` is set using `forkchoiceUpdated`. Eth RPC should return block set on the `SafeBlockHash`. When no block has been set as `SafeBlockHash` (`SafeBlockHash==0x00..00`) or PoW mode still, using `safe` shall return error.

- `finalized` Block after New FinalizedBlockHash:  
Verify the Block returned by the Eth RPC using the `finalized` keyword after a new `FinalizedBlockHash` is set using `forkchoiceUpdated`. Eth RPC should return block set on the `FinalizedBlockHash`. When no block has been set as `FinalizedBlockHash` (`FinalizedBlockHash==0x00..00`) or PoW mode still, using `finalized` shall return error.

- `latest` Block after Reorg:  
Verify the Block returned by the Eth RPC after a forkchoiceUpdated reorgs HeadBlockHash/SafeBlockHash to a sidechain and back. Eth RPC should return the appropriate block everytime.

### Payload Execution
- Re-Execute Payload:  
Re-execute already executed payloads (10) and verify that no errors occur.

- Multiple New Payloads Extending Canonical Chain:  
Send 80 valid NewPayload directives extending the canonical chain. Only one of the payloads is selected using ForkchoiceUpdated directive.

- Out of Order Payload Execution:  
Launch a first client and produce N payloads.  
Launch a second client and send payloads (NewPayload) in reverse order (N, N - 1, ..., 1).  
The payloads should be ACCEPTED/SYNCING, and the last payload should be VALID (since payload 1 successfully links the chain with the Genesis).

- Valid NewPayload->ForkchoiceUpdated on Syncing Client:
Skip sending NewPayload to the client, but send the ForkchoiceUpdated to this missing payload, which will send the client to Syncing, then send the valid payload. Response should be either `ACCEPTED` or `SYNCING`.

- NewPayload with Missing ForkchoiceUpdated
Chain `Genesis <- ... <- TB <- P1 <- P2 <- ... <- Pn`, `TB` is a valid terminal block. Secondary EL starts with `TB` as the head. `newPayload(P1)`, `newPayload(P2)`, ... `newPayload(Pn)` are sent to the secondary EL, all responses shall be `{status: VALID, latestValidHash: payload.blockHash}`. `forkchoiceUpdated(head: Pn, safe: Pn-1, finalized:Pn-2)` is sent and verified.

### Invalid Payload Tests
- Bad blockhash on NewPayload:  
Send a NewPayload directive to the client including an incorrect BlockHash, should result in an error in all the following cases:
   - NewPayload while not syncing, on canonical chain
   - NewPayload while not syncing, on side chain
   - NewPayload while syncing, on canonical chain
   - NewPayload while syncing, on side chain

- ParentHash==BlockHash on NewPayload:  
Send a NewPayload directive to the client including ParentHash that is equal to the BlockHash (Incorrect hash).

- Invalid Transition Payload:
Build canonical chain `A: Genesis <- ... <- TB <- P1 <- P2` where TB is a valid terminal block.
Create an invalid transition payload `INV_P1` with parent `TB`.
`newPayload(INV_P1)` must return `{status: INVALID/ACCEPTED, latestValidHash: 0x00..00}`.
`forkchoiceUpdated(head: INV_P1, safe: 0x00..00, finalized: 0x00..00)` must return `{status: INVALID, latestValidHash: 0x00..00}`.

- Invalid Field in NewPayload:  
Send an invalid payload in NewPayload by modifying fields of a valid ExecutablePayload while maintaining a valid BlockHash.
After attempting to NewPayload/ForkchoiceUpdated the invalid payload, also attempt to send a valid payload that contains the previously modified invalid payload as parent (should also fail).
Test also has variants with a missing parent payload (client is syncing):
I.e. Skip sending NewPayload to the client, but send the ForkchoiceUpdated to this missing payload, which will send the client to Syncing, then send the invalid payload.
Modify fields including:
   - ParentHash
   - StateRoot
   - ReceiptsRoot
   - BlockNumber
   - GasLimit
   - GasUsed
   - Timestamp
   - PrevRandao
   - Removing a Transaction
   - Transaction with incorrect fields:
      - Signature
      - Nonce
      - GasPrice
      - Gas
      - Value

- Invalid Ancestor Re-Org Tests
Attempt to re-org to an unknown side chain which at some point contains an invalid payload.
The side chain is constructed in parallel while the CL Mock builds the canonical chain, but changing the extraData to simply produce a different hash.
At a given point, the side chain invalidates one of the payloads by modifying one of the payload fields.
Once the side chain reaches a certain deviation height (N) from the commonAncestor, the CL switches to it by either of the following methods:
a) `newPayload` each of the payloads in the side chain, and the invalid payload shall return `INVALID`
b) Force the main client to partially sync the side chain by sending the initial slice of the chain (before the invalid payload) to another client, and the rest to the primary client.
Method (b) results in the client returning `ACCEPTED` or `SYNCING` on the `newPayload(INV_P)`, but eventually the client must return `INVALID` to the head of the side chain because it was built on top of an invalid payload.
```
commonAncestor◄─▲── P1 ◄─ P2 ◄─ P3 ◄─ ... ◄─ Pn
		          │
		          └── P1' ◄─ P2' ◄─ ... ◄─ INV_P ◄─ ... ◄─ Pn'
```


### Re-org using Engine API
- Transaction Reorg using ForkchoiceUpdated:  
Send transactions that modify the state tree after the PoS switch and verify that the modifications are correctly rolled back when a ForkchoiceUpdated event occurs with a block on a different chain than the block where the transaction was included.

- Sidechain Reorg:  
Send a transaction that modifies the state, ForkchoiceUpdate to the payload containing this transaction. Then send an alternative payload, that produces a different state with the same transaction, and use ForkchoiceUpdated to change into this sidechain.

- Re-Org Back into Canonical Chain:  
Test that performing a re-org back into a previous block of the canonical chain does not produce errors and the chain is still capable of progressing.

- Re-Org Back to Canonical Chain From Syncing Chain:
Build an alternative chain of 10 payloads.
Perform `newPayload(P10')` + `fcU(P10')`, which should result in client `SYNCING`. Verify that the client can re-org back to the canonical chain after sending `newPayload(P11)` + `fcU(P11)`.

### Suggested Fee Recipient in Payload creation
- Suggested Fee Recipient Test:  
Set the fee recipient to a custom address and verify that (a) balance is not increased when no fees are collected (b) balance is increased appropriately when fees are collected.

### PrevRandao Opcode:
- PrevRandao Opcode Transactions:  
Send transactions that modify the state to the value of the 'DIFFICULTY' opcode and verify that:  
(a) the state is equal to the difficulty on blocks before the TTD is crossed  
(b) the state is equal to the PrevRandao value provided using forkchoiceUpdated after PoS transition.  

### Sync Tests:
- Sync Client Post Merge:  
Launch a first client and verify that the transition to PoS occurs.  
Launch a second client and verify that it successfully syncs to the first client's chain.

## Engine API Merge Tests:

Test cases using multiple Proof of Work chains to test the client's behavior when the Terminal Total Difficulty is reached by two different blocks simultaneously and the Engine API takes over.

- Single Block PoW Re-org to Higher-Total-Difficulty Chain, Equal Height:  
Client 1 starts with chain G <- A, Client 2 starts with chain G <- B.  
Blocks A and B reach TTD, but block B has higher difficulty than A.  
ForkchoiceUpdated is sent to Client 1 with A as Head.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
Verification is made that Client 1 Re-orgs to chain G <- B.  

- Single Block PoW Re-org to Higher-Total-Difficulty Chain, Equal Height (Transition Payload):  
Client 1 starts with chain G <- A, Client 2 starts with chain G <- B.  
Blocks A and B reach TTD, but block B has higher difficulty than A.  
ForkchoiceUpdated is sent to Client 1 with A as Head.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
P1 is produced by Client 2 on top of G <- B.
`newPayload(P1)` is sent to Client 1 and verification is made that `{status: VALID, latestValidHash: P1.BlockHash}`.

- Single Block PoW Re-org to Lower-Total-Difficulty Chain, Equal Height:  
Client 1 starts with chain G <- A, Client 2 starts with chain G <- B.  
Blocks A and B reach TTD, but block A has higher difficulty than B.  
ForkchoiceUpdated is sent to Client 1 with A as Head.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
Verification is made that Client 1 Re-orgs to chain G <- B.  

- Two Block PoW Re-org to Higher-Total-Difficulty Chain, Equal Height:  
Client 1 starts with chain G <- A <- B, Client 2 starts with chain G <- C <- D.  
Blocks B and D reach TTD, but block D has higher total difficulty than B.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
ForkchoiceUpdated is sent to Client 1 with D as Head.  
Verification is made that Client 1 Re-orgs to chain G <- C <- D.  

- Two Block PoW Re-org to Lower-Total-Difficulty Chain, Equal Height:  
Client 1 starts with chain G <- A <- B, Client 2 starts with chain G <- C <- D.  
Blocks B and D reach TTD, but block B has higher total difficulty than D.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
ForkchoiceUpdated is sent to Client 1 with D as Head.  
Verification is made that Client 1 Re-orgs to chain G <- C <- D.  

- Two Block PoW Re-org to Higher-Height Chain:  
Client 1 starts with chain G <- A, Client 2 starts with chain G <- B <- C.  
Blocks A and C reach TTD, but block C has higher height than A.  
ForkchoiceUpdated is sent to Client 1 with A as Head.  
ForkchoiceUpdated is sent to Client 1 with C as Head.  
Verification is made that Client 1 Re-orgs to chain G <- B <- C.  

- Two Block PoW Re-org to Lower-Height Chain:  
Client 1 starts with chain G <- A <- B, Client 2 starts with chain G <- C.  
Blocks B and C reach TTD, but block B has higher height than C.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
ForkchoiceUpdated is sent to Client 1 with C as Head.  
Verification is made that Client 1 Re-orgs to chain G <- C.  

- Two Block PoW Re-org to Lower-Height Chain, Transaction Overwrite
Client 1 starts with chain G <- A <- B, Client 2 starts with chain G <- C. 
Blocks B and C reach TTD, but block B has higher height than C.  
Block A and C contain TX1, and Block B contains TX2.
TX1 and TX2 use the DIFFICULTY/PREVRANDAO opcode into storage. 
ForkchoiceUpdated is sent to Client 1 with B as Head.  
ForkchoiceUpdated is sent to Client 1 with C as Head.  
Verification is made that Client 1 Re-orgs to chain G <- C.  
PoS chain on top of C contains TX2, which was originally part of the PoW chain.
Verification is made that the resulting PoS storage contains PREVRANDAO instead of DIFFICULTY.

- Two Block Post-PoS Re-org to Higher-Total-Difficulty PoW Chain:
Client 1 starts with chain G <- A.
ForkchoiceUpdated is sent to Client 1 with A as Head.
Two PoS blocks are produced on top of A.
Client 2 starts with chain G <- B.  
Blocks A and B reach TTD, but block B has higher difficulty than A.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
PoS Chain is continued on top of B.
Verification is made that Client 1 Re-orgs to chain G <- B <- ... 

- Two Block Post-PoS Re-org to Lower-Total-Difficulty PoW Chain:
Client 1 starts with chain G <- A.
ForkchoiceUpdated is sent to Client 1 with A as Head.
Two PoS blocks are produced on top of A.
Client 2 starts with chain G <- B.  
Blocks A and B reach TTD, but block A has higher difficulty than B.  
ForkchoiceUpdated is sent to Client 1 with B as Head.  
PoS Chain is continued on top of B.
Verification is made that Client 1 Re-orgs to chain G <- B <- ... 

- Two Block Post-PoS Re-org to Higher-Height PoW Chain:
Client 1 starts with chain G <- A.
ForkchoiceUpdated is sent to Client 1 with A as Head.
Two PoS blocks are produced on top of A.
Client 2 starts with chain G <- B <- C.  
Blocks A and C reach TTD, but have different heights.  
ForkchoiceUpdated is sent to Client 1 with C as Head.  
PoS Chain is continued on top of C.
Verification is made that Client 1 Re-orgs to chain G <- B <- C <- ...  

- Two Block Post-PoS Re-org to Lower-Height PoW Chain:
Client 1 starts with chain G <- A <- B.
ForkchoiceUpdated is sent to Client 1 with B as Head.
Two PoS blocks are produced on top of B.
Client 2 starts with chain G <- C.  
Blocks B and C reach TTD, but have different heights.  
ForkchoiceUpdated is sent to Client 1 with C as Head.  
PoS Chain is continued on top of C.
Verification is made that Client 1 Re-orgs to chain G <- C <- ...  

- Transition to a Chain with Invalid Terminal Block, Incorrectly Configured Higher Total Difficulty
Client 1 starts with chain G <- A, Client 2 starts with chain G <- A <- B.  
Client 1's configured TTD is reached by A.
Client 2's configured TTD is reached by B.
ForkchoiceUpdated is sent to both clients with B as Head.  
PoS chain is continued on top of B.
Verification is made that Client 1 never re-orgs to chain G <- A <- B due to incorrect Terminal block.

- Transition to a Chain with Invalid Terminal Block, Incorrectly Configured Higher Total Difficulty (Transition Payload)
Client 1 starts with chain G <- A, Client 2 starts with chain G <- A <- B.  
Client 1's configured TTD is reached by A.
Client 2's configured TTD is reached by B.
ForkchoiceUpdated is sent to both clients with B as Head.  
P1 is prepared by Client 2 on top of G <- A <- B.
`newPayload(P1)` is sent to Client 1 and verification is made that `{status: INVALID, latestValidHash: 0x00..00}`

- Transition to a Chain with Invalid Terminal Block, Incorrectly Configured Lower Total Difficulty
Client 1 starts with chain G <- A <- B, Client 2 starts with chain G <- A.  
Client 1's configured TTD is reached by B.
Client 2's configured TTD is reached by A.
ForkchoiceUpdated is sent to both clients with A as Head.  
PoS chain is continued on top of A.
Verification is made that Client 1 never re-orgs to chain G <- A <- ... due to incorrect Terminal block. 

- Transition to a Chain with Invalid Terminal Block, Incorrectly Configured Lower Total Difficulty (Transition Payload)
Client 1 starts with chain G <- A <- B, Client 2 starts with chain G <- A.  
Client 1's configured TTD is reached by B.
Client 2's configured TTD is reached by A.
ForkchoiceUpdated is sent to both clients with A as Head.  
P1 is prepared by Client 2 on top of G <- A.
`newPayload(P1)` is sent to Client 1 and verification is made that `{status: INVALID, latestValidHash: 0x00..00}`

- Halt following PoW chain:  
 Client 1 starts with chain G <- A, Client 2 starts with chain G <- A <- B.  
 Block A reaches TTD, but Client 2 has a higher TTD and accepts block B (simulating a client not complying with the merge).  
 Verification is made that Client 1 does not follow Client 2 chain to block B.  

- Long PoW Chain Sync:
Client 1 starts with chain G <- PoW1, Client 2 starts with chain G <- PoW1 <- ... <- PoW1024.
Block PoW1024 reaches TTD, and the CL Mock continues the PoS chain on top of this block.
Verification is made that Client 1 syncs the remaining PoW blocks and also the PoS chain built on top of PoW1024.

## JWT Authentication Tests:
- No time drift, correct secret:  
Engine API call where the `iat` claim contains no time drift, and the secret to calculate the token is correct.
No error is expected.

- No time drift, incorrect secret (shorter):  
Engine API call where the `iat` claim contains no time drift, but the secret to calculate the token is incorrectly shorter.
Invalid token error is expected.

- No time drift, incorrect secret (longer):  
Engine API call where the `iat` claim contains no time drift, but the secret to calculate the token is incorrectly longer.
Invalid token error is expected.

- Negative time drift, exceeding limit, correct secret:  
Engine API call where the `iat` claim contains a negative time drift greater than the maximum threshold, but the secret to calculate the token is correct.
Invalid token error is expected.

- Negative time drift, within limit, correct secret:  
Engine API call where the `iat` claim contains a negative time drift smaller than the maximum threshold, and the secret to calculate the token is correct.
No error is expected.

- Positive time drift, exceeding limit, correct secret:  
Engine API call where the `iat` claim contains a positive time drift greater than the maximum threshold, but the secret to calculate the token is correct.
Invalid token error is expected.

- Positive time drift, within limit, correct secret:  
Engine API call where the `iat` claim contains a positive time drift smaller than the maximum threshold, and the secret to calculate the token is correct.
No error is expected.