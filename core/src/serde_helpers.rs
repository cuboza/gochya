use serde::{Deserialize, Serialize};

pub const SCHEMA_VERSION: u32 = 1;

/// Versioned envelope for persistent domain values.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SchemaEnvelope<T> {
    pub schema_version: u32,
    pub payload: T,
}

impl<T> SchemaEnvelope<T> {
    #[must_use]
    pub const fn current(payload: T) -> Self {
        Self {
            schema_version: SCHEMA_VERSION,
            payload,
        }
    }
}
