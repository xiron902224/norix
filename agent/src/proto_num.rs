use serde::de::{self, Deserializer, Visitor};
use std::fmt;

/// Protobuf JSON encodes int64 as strings; accept string, number, or null.
pub fn deserialize_i64<'de, D>(deserializer: D) -> Result<i64, D::Error>
where
    D: Deserializer<'de>,
{
    struct I64Visitor;

    impl<'de> Visitor<'de> for I64Visitor {
        type Value = i64;

        fn expecting(&self, formatter: &mut fmt::Formatter) -> fmt::Result {
            formatter.write_str("an i64, stringified i64, or null")
        }

        fn visit_unit<E: de::Error>(self) -> Result<i64, E> {
            Ok(0)
        }

        fn visit_none<E: de::Error>(self) -> Result<i64, E> {
            Ok(0)
        }

        fn visit_i64<E: de::Error>(self, value: i64) -> Result<i64, E> {
            Ok(value)
        }

        fn visit_u64<E: de::Error>(self, value: u64) -> Result<i64, E> {
            i64::try_from(value).map_err(E::custom)
        }

        fn visit_f64<E: de::Error>(self, value: f64) -> Result<i64, E> {
            Ok(value as i64)
        }

        fn visit_str<E: de::Error>(self, value: &str) -> Result<i64, E> {
            if value.is_empty() {
                return Ok(0);
            }
            value.parse().map_err(E::custom)
        }
    }

    deserializer.deserialize_any(I64Visitor)
}

pub fn deserialize_i32<'de, D>(deserializer: D) -> Result<i32, D::Error>
where
    D: Deserializer<'de>,
{
    struct I32Visitor;

    impl<'de> Visitor<'de> for I32Visitor {
        type Value = i32;

        fn expecting(&self, formatter: &mut fmt::Formatter) -> fmt::Result {
            formatter.write_str("an i32, stringified i32, or null")
        }

        fn visit_unit<E: de::Error>(self) -> Result<i32, E> {
            Ok(0)
        }

        fn visit_none<E: de::Error>(self) -> Result<i32, E> {
            Ok(0)
        }

        fn visit_i64<E: de::Error>(self, value: i64) -> Result<i32, E> {
            i32::try_from(value).map_err(E::custom)
        }

        fn visit_u64<E: de::Error>(self, value: u64) -> Result<i32, E> {
            i32::try_from(value).map_err(E::custom)
        }

        fn visit_f64<E: de::Error>(self, value: f64) -> Result<i32, E> {
            Ok(value as i32)
        }

        fn visit_str<E: de::Error>(self, value: &str) -> Result<i32, E> {
            if value.is_empty() {
                return Ok(0);
            }
            value.parse().map_err(E::custom)
        }
    }

    deserializer.deserialize_any(I32Visitor)
}
