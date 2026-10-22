# Reviewed public CA proof fixture

`reviewed-ca-bundle.pem` is the existing Phase 1 approved CA candidate, not a new trust store or runtime configuration. It contains public certificates and OpenSSL trust metadata, no private keys or credentials. SHA256: `3ba2000fe3d55cb6c41323fabbb105743e2c9422151eea213311538b07b23f5a`; size: 472,033 bytes.

It was reproduced without installing packages from nixpkgs revision `b7c2ada94fe99c15b0dbcf4d11fd7850b957a436`:

- [NSS cacert derivation](https://github.com/NixOS/nixpkgs/blob/b7c2ada94fe99c15b0dbcf4d11fd7850b957a436/pkgs/by-name/ca/cacert/package.nix) pins NSS 3.126 `certdata.txt`, SHA256 (base64) `gbfyV2MzouNg5nP5Etewt6dl2DbHMQA+NIpGysXTcZg=`. Source: [NSS_3_126_RTM](https://github.com/nss-dev/nss/blob/NSS_3_126_RTM/lib/ckfw/builtins/certdata.txt); upstream license [MPL-2.0](https://github.com/nss-dev/nss/blob/NSS_3_126_RTM/COPYING).
- [buildcatrust derivation](https://github.com/NixOS/nixpkgs/blob/b7c2ada94fe99c15b0dbcf4d11fd7850b957a436/pkgs/development/python-modules/buildcatrust/default.nix) pins MIT-licensed buildcatrust 0.5.1 source distribution, SHA256 (base64) `I7f2LCQ8dGFOX/d04mOUll7IL7y5Qn1EPu9UO5496So=`. Its unchanged Python sources use only the standard library for this operation.

Both complete source hashes were verified before source extraction/execution. With those authenticated inputs, the unchanged builder invocation was:

```sh
python3 -m buildcatrust.cli --certdata_input certdata.txt --ca_bundle_output ca-bundle.crt
```

The resulting `ca-bundle.crt` matched the already approved byte digest exactly. No certificates were generated, and no host/user trust, package lock, credential, or access setting changed. The fixture has 121 ordinary server-auth certificates and 172 OpenSSL TRUSTED CERTIFICATE records; 51 AUX-only certificates must stay outside the Go TLS pool. This is reproducible input and local parser evidence, not qualified-host acquisition evidence or a support claim. Retention/disposition is governed by the `internal/proof/` row in `docs/roadmap/hard-cutover.md`.
