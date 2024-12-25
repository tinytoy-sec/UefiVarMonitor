// Copyright © 2019 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Inspired by https://github.com/phil-opp/blog_os/blob/post-03/src/vga_buffer.rs
// from Philipp Oppermann

use core::fmt;

use atomic_refcell::AtomicRefCell;
use x86_64::instructions::port::PortWriteOnly;

// 定义串口地址常量
const SERIAL_PORT_ADDRESS: u16 = 0x3f8;

// 使用 const 初始化静态变量
static PORT: AtomicRefCell<PortWriteOnly<u8>> = AtomicRefCell::new(PortWriteOnly::new(SERIAL_PORT_ADDRESS));

pub struct Serial;

impl Serial {
    // 添加一个安全的写入方法
    #[inline]
    pub fn write_byte(byte: u8) {
        let mut port = PORT.borrow_mut();
        unsafe { port.write(byte) }
    }

    // 添加一个批量写入方法
    pub fn write_bytes(bytes: &[u8]) {
        let mut port = PORT.borrow_mut();
        for &byte in bytes {
            unsafe { port.write(byte) }
        }
    }
}

impl fmt::Write for Serial {
    fn write_str(&mut self, s: &str) -> fmt::Result {
        Serial::write_bytes(s.as_bytes());
        Ok(())
    }
}

#[macro_export]
macro_rules! log {
    ($($arg:tt)*) => {{
        use core::fmt::Write;
        #[cfg(all(feature = "log-serial", not(test)))]
        {
            let _ = writeln!($crate::serial::Serial, $($arg)*);
        }
        #[cfg(all(feature = "log-serial", test))]
        println!($($arg)*);
    }};
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_log_macro() {
        log!("Test message");
        log!("Formatted {}", "string");
    }
}
