# Distributed Key Generation (DKG) 

This is an implementation of DKG over Ethereum Swarm. As a result any number of users will get their BLS key pairs with any predefined threshold.
Swarm is used as a transport layer with an additional `Stream` abstraction: a couple of feeds that a user wants to read and hit own feed to send new messages.

# Verifiable distributed source of randomness
BLS signatures are user to implement simple but efficient VRF.

## Idea
The main idea of the project is to try PoC of DKG and VRF not depending of blockchain state and gas usage. It could be used in decentralized gaming.

## Test
There is a test `TestDKG` that implements an example of random number generation using swarm test server. It could be run by `make run`