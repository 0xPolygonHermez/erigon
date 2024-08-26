use plonky2::field::goldilocks_field::GoldilocksField;
use plonky2::hash::poseidon::{PoseidonHash};
use plonky2::hash::hash_types::HashOut;
use plonky2::field::types::{Field, PrimeField64};
use plonky2::plonk::config::{GenericHashOut, Hasher};
use jemallocator::Jemalloc;

#[global_allocator]
static GLOBAL: Jemalloc = Jemalloc;

#[repr(C)]
pub struct Buffer {
    data: *mut u64,
    len: usize,
}

#[no_mangle]
pub extern fn rustPoseidongoldHash(input_ptr: Buffer) -> Buffer {
    let input = unsafe {
        std::slice::from_raw_parts_mut(input_ptr.data, input_ptr.len)
    };

    let inputs:Vec<GoldilocksField>= input.into_iter()
        .map(|x| {GoldilocksField::from_canonical_u64(*x)})
        .collect();

    // Calculate the Poseidon hash using Plonky2
    let hash_output: HashOut<GoldilocksField> = PoseidonHash::hash_result_scalar(&inputs);

    let transformed_hash_vec: Vec<u64> = hash_output.to_vec().into_iter()
        .map(|x|  GoldilocksField::to_canonical_u64(&x))
        .collect();

    let mut buf = transformed_hash_vec.into_boxed_slice();
    let data = buf.as_mut_ptr();
    let len = buf.len();

    std::mem::forget(buf);

    Buffer { data, len }
}

#[no_mangle]
pub extern fn free_buf(buf: Buffer) {
    if !buf.data.is_null() {
        unsafe {
            let _ = Box::from_raw(std::slice::from_raw_parts_mut(buf.data, buf.len));
        }
    }
}