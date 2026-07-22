// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

interface IWorkerSpecifier {
    function workerHash() external view returns (bytes32);
    function workerResolvers() external view returns (string[] memory);
}

contract ZKNWalletShield is IWorkerSpecifier {
    bytes32 public immutable hash;
    string[] public resolvers;

    constructor(bytes32 _hash, string[] memory _resolvers) {
        hash = _hash;
        resolvers = _resolvers;
    }

    function workerHash() external view returns (bytes32) {
        return hash;
    }

    function workerResolvers() external view returns (string[] memory) {
        return resolvers;
    }
}
