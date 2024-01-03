# L1 Info Tree

L1InfoTree has 32 static levels. It is a sparse merkle tree of information pertaining to the L1.

- Incremental
- Add new leaf when a new GER is computed in the SC ● Given an index → could always get (verify) a L1Data
- If an index does not still contain any L1Data → L1Data = 0
- Only save the latest l1InfoRoot in the SC
- Index 0 has GER = 0 (Special index)
- Allow not changing GER with index = 0
- Less gas cost data-availability