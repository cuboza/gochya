use std::{env, fs, path::PathBuf};

fn main() {
    println!("cargo:rerun-if-changed=src/ffi.rs");
    println!("cargo:rerun-if-changed=cbindgen.toml");
    println!("cargo:rerun-if-env-changed=GOCHYA_UPDATE_HEADER");

    let crate_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("manifest directory"));
    let config =
        cbindgen::Config::from_file(crate_dir.join("cbindgen.toml")).expect("valid cbindgen.toml");
    let bindings = cbindgen::Builder::new()
        .with_crate(&crate_dir)
        .with_config(config)
        .generate()
        .expect("generate C header");
    let generated_path =
        PathBuf::from(env::var("OUT_DIR").expect("Cargo OUT_DIR")).join("gochya_core.h");
    bindings.write_to_file(&generated_path);

    let committed_path = crate_dir.join("ffi/gochya_core.h");
    if env::var_os("GOCHYA_UPDATE_HEADER").is_some() {
        fs::create_dir_all(committed_path.parent().expect("header parent"))
            .expect("create ffi directory");
        fs::copy(&generated_path, &committed_path).expect("update committed C header");
    } else if committed_path.exists() {
        let generated = fs::read(&generated_path).expect("read generated C header");
        let committed = fs::read(&committed_path).expect("read committed C header");
        assert_eq!(
            generated, committed,
            "core/ffi/gochya_core.h is stale; run GOCHYA_UPDATE_HEADER=1 cargo build -p gochya-core"
        );
    }
}
