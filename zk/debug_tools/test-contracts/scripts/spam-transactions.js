async function main() {
    try {
        // If no %%url%% is provided, it connects to the default
        provider = hre.ethers.provider

        // Get write access as an account by getting the signer
        signer = await provider.getSigner()

        nonce = await provider.getTransactionCount("0x67b1d87101671b127f5f8714789C7192f7ad340e")
        console.log("Nonce: ", nonce)

        txs = []
        for (let i = nonce+999; i >= nonce; i--) {
            tx = await signer.sendTransaction({
                to: "0x67b1d87101671b127f5f8714789C7192f7ad340e",
                value: hre.ethers.parseEther("0.0000000000"+i),
                gasPrice: 1000000,
                nonce: i,
            });
        }          
        // console.log(tx)
        // Often you may wish to wait until the transaction is mined
        // receipt = await tx.wait();
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