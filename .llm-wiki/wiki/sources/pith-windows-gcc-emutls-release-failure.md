---
type: source
title: Windows GCC emutls release failure
status: insight
category: release
created: 2026-09-16
updated: 2026-09-16
slug: pith-windows-gcc-emutls-release-failure
---

# Windows GCC emutls release failure

AIDEV-270: v2.4.1 run 35062506835 failed on Windows GCC 15.2 due to duplicate __emutls_v._ZSt11__once_call and __emutls_v._ZSt15__once_callable definitions: libstdc++ mutex.o and pkg/anomaly/emutls_stub_windows.c. Existing shim was introduced for GCC16. Fix scopes it to Windows GCC16 excluding Clang. Local GCC16 full tests/build passed; compiler-macro object checks showed zero shim symbols for GCC15 and two for GCC16 (not a substitute for native GCC15 CI). Added Windows PR build check. Next release uses v2.4.2; do not move failed v2.4.1 tag. Await human merge and successful published signed assets before local release upgrade.

*Category: release*

---
*Captured: 2026-09-16*

## Related

_Add links to related pages._
