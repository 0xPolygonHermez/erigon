// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Burnable.sol";

contract MyToken is ERC20Burnable {
    address payable public owner;
    uint256 public blockReward;

    constructor(
        uint256 cap,
        uint256 reward
    ) ERC20("MyToken", "MYT") {
        owner = payable(msg.sender);
        _mint(owner, 50000000 * (10 ** decimals()));
        blockReward = reward * (10 ** decimals()); // Setting block reward for first deploy
    }

}