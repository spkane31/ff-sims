#[no_mangle]
pub extern "C" fn add_one(x: i32) -> i32 {
    x + 1
}

#[no_mangle]
pub extern "C" fn minus_one(x: i32) -> i32 {
    x - 1
}

#[no_mangle]
pub extern "C" fn simulation() -> i32 {
    return 10 as i32;
}
