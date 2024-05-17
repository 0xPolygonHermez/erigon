async function main() {
try {
   // Get the ContractFactory of your SimpleContract
   const SimpleContract = await hre.ethers.getContractFactory("ModxContract");

   // Deploy the contract
   const contract = await SimpleContract.deploy();
   // Wait for the deployment transaction to be mined
   await contract.waitForDeployment();

   console.log(`SimpleContract deployed to: ${await contract.getAddress()}`);

   const result = await contract.mulmod();
   console.log(result);
 } catch (error) {
   console.error(error);
   process.exit(1);
 }
}

main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(error);
    process.exit(1);
  });