/* GCC 16's MinGW runtime no longer supplies these legacy emutls symbols
 * required by the DuckDB static bindings. GCC 15 still defines them in
 * libstdc++ (mutex.o), so emitting the shim there causes duplicate symbols.
 * Keep this workaround scoped to the compiler version it was validated on.
 */
#if defined(_WIN32) && defined(__GNUC__) && __GNUC__ == 16 && !defined(__clang__)
__asm__(
".globl __emutls_v._ZSt11__once_call\n"
".globl __emutls_v._ZSt15__once_callable\n"
".data\n"
"__emutls_v._ZSt11__once_call:\n"
"    .quad 0, 0, 0, 0\n"
"__emutls_v._ZSt15__once_callable:\n"
"    .quad 0, 0, 0, 0\n"
);
#endif
