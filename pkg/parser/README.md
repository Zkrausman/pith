# parser

This package contains all specialized optimizers (parsers) that compress terminal output for specific CLIs and tools.

```mermaid
graph TD
    Interface["Parser Interface [pkg/parser/interface.go]"]
    Git["GitParsers [pkg/parser/git.go]"]
    Thneed["ThneedParser [pkg/parser/thneed.go]"]
    NPM["NPMParser [pkg/parser/npm.go]"]
    Node["NodeParser [pkg/parser/node.go]"]
    FS["FS/LS Parser [pkg/parser/fs.go]"]
    GitHub["GitHubParser [pkg/parser/infra.go]"]
    
    Interface <|-- Git
    Interface <|-- Thneed
    Interface <|-- NPM
    Interface <|-- Node
    Interface <|-- FS
    Interface <|-- GitHub
```


## Architecture

<!-- mermaid-start -->
```mermaid
graph TD
    e__repos_pith_pkg_parser_readme_md["[MODULE] /repos/pith/pkg/parser/readme.md [readme.md]"]
    e__repos_pith_pkg_parser_bd_go["[MODULE] /repos/pith/pkg/parser/bd.go [bd.go]"]
    e__repos_pith_pkg_parser_bd_go_bdparser["[STRUCT] bdparser [bd.go]"]
    e__repos_pith_pkg_parser_bd_go_name["[FUNCTION] name [bd.go]"]
    e__repos_pith_pkg_parser_bd_go_canparse["[FUNCTION] canparse [bd.go]"]
    e__repos_pith_pkg_parser_bd_go_parse["[FUNCTION] parse [bd.go]"]
    e__repos_pith_pkg_parser_chain_go["[MODULE] /repos/pith/pkg/parser/chain.go [chain.go]"]
    e__repos_pith_pkg_parser_chain_go_splitsubcommands["[FUNCTION] splitsubcommands [chain.go]"]
    e__repos_pith_pkg_parser_chain_go_chainparser["[STRUCT] chainparser [chain.go]"]
    e__repos_pith_pkg_parser_chain_go_name["[FUNCTION] name [chain.go]"]
    e__repos_pith_pkg_parser_chain_go_canparse["[FUNCTION] canparse [chain.go]"]
    e__repos_pith_pkg_parser_chain_go_parse["[FUNCTION] parse [chain.go]"]
    e__repos_pith_pkg_parser_fs_go["[MODULE] /repos/pith/pkg/parser/fs.go [fs.go]"]
    e__repos_pith_pkg_parser_fs_go_lsparser["[STRUCT] lsparser [fs.go]"]
    e__repos_pith_pkg_parser_fs_go_name["[FUNCTION] name [fs.go]"]
    e__repos_pith_pkg_parser_fs_go_canparse["[FUNCTION] canparse [fs.go]"]
    e__repos_pith_pkg_parser_fs_go_parse["[FUNCTION] parse [fs.go]"]
    e__repos_pith_pkg_parser_fs_go_findparser["[STRUCT] findparser [fs.go]"]
    e__repos_pith_pkg_parser_fs_go_treeparser["[STRUCT] treeparser [fs.go]"]
    e__repos_pith_pkg_parser_fs_go_duparser["[STRUCT] duparser [fs.go]"]
    e__repos_pith_pkg_parser_fs_test_go["[MODULE] /repos/pith/pkg/parser/fs_test.go [fs_test.go]"]
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser["[FUNCTION] testtreeparser [fs_test.go]"]
    e__repos_pith_pkg_parser_fs_test_go_testduparser["[FUNCTION] testduparser [fs_test.go]"]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser["[FUNCTION] testlsparser [fs_test.go]"]
    e__repos_pith_pkg_parser_fs_test_go_testfindparser["[FUNCTION] testfindparser [fs_test.go]"]
    e__repos_pith_pkg_parser_get_content_test_go["[MODULE] /repos/pith/pkg/parser/get_content_test.go [get_content_test.go]"]
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser["[FUNCTION] testgetcontentparser [get_content_test.go]"]
    e__repos_pith_pkg_parser_git_go["[MODULE] /repos/pith/pkg/parser/git.go [git.go]"]
    e__repos_pith_pkg_parser_git_go_name["[FUNCTION] name [git.go]"]
    e__repos_pith_pkg_parser_git_go_canparse["[FUNCTION] canparse [git.go]"]
    e__repos_pith_pkg_parser_git_go_gitlogparser["[STRUCT] gitlogparser [git.go]"]
    e__repos_pith_pkg_parser_git_go_formatcommit["[FUNCTION] formatcommit [git.go]"]
    e__repos_pith_pkg_parser_git_go_gitdiffparser["[STRUCT] gitdiffparser [git.go]"]
    e__repos_pith_pkg_parser_git_go_gitbranchparser["[STRUCT] gitbranchparser [git.go]"]
    e__repos_pith_pkg_parser_git_go_compositegitparser["[STRUCT] compositegitparser [git.go]"]
    e__repos_pith_pkg_parser_git_go_gitstatusparser["[STRUCT] gitstatusparser [git.go]"]
    e__repos_pith_pkg_parser_git_go_parse["[FUNCTION] parse [git.go]"]
    e__repos_pith_pkg_parser_git_test_go["[MODULE] /repos/pith/pkg/parser/git_test.go [git_test.go]"]
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser["[FUNCTION] testgitstatusparser [git_test.go]"]
    e__repos_pith_pkg_parser_git_test_go_testgitlogparser["[FUNCTION] testgitlogparser [git_test.go]"]
    e__repos_pith_pkg_parser_git_test_go_testgitdiffparser["[FUNCTION] testgitdiffparser [git_test.go]"]
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser["[FUNCTION] testgitbranchparser [git_test.go]"]
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser["[FUNCTION] testcompositegitparser [git_test.go]"]
    e__repos_pith_pkg_parser_github_release_go["[MODULE] /repos/pith/pkg/parser/github_release.go [github_release.go]"]
    e__repos_pith_pkg_parser_github_release_go_githubreleaseparser["[STRUCT] githubreleaseparser [github_release.go]"]
    e__repos_pith_pkg_parser_github_release_go_name["[FUNCTION] name [github_release.go]"]
    e__repos_pith_pkg_parser_github_release_go_canparse["[FUNCTION] canparse [github_release.go]"]
    e__repos_pith_pkg_parser_github_release_go_parse["[FUNCTION] parse [github_release.go]"]
    e__repos_pith_pkg_parser_go_go["[MODULE] /repos/pith/pkg/parser/go.go [go.go]"]
    e__repos_pith_pkg_parser_go_go_name["[FUNCTION] name [go.go]"]
    e__repos_pith_pkg_parser_go_go_canparse["[FUNCTION] canparse [go.go]"]
    e__repos_pith_pkg_parser_go_go_parse["[FUNCTION] parse [go.go]"]
    e__repos_pith_pkg_parser_go_go_goparser["[STRUCT] goparser [go.go]"]
    e__repos_pith_pkg_parser_infra_go["[MODULE] /repos/pith/pkg/parser/infra.go [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_parse["[FUNCTION] parse [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_dockerpsparser["[STRUCT] dockerpsparser [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_dependencyparser["[STRUCT] dependencyparser [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_testparser["[STRUCT] testparser [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_githubparser["[STRUCT] githubparser [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_envparser["[STRUCT] envparser [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_name["[FUNCTION] name [infra.go]"]
    e__repos_pith_pkg_parser_infra_go_canparse["[FUNCTION] canparse [infra.go]"]
    e__repos_pith_pkg_parser_infra_test_go["[MODULE] /repos/pith/pkg/parser/infra_test.go [infra_test.go]"]
    e__repos_pith_pkg_parser_infra_test_go_testenvparser["[FUNCTION] testenvparser [infra_test.go]"]
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser["[FUNCTION] testdockerpsparser [infra_test.go]"]
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser["[FUNCTION] testdependencyparser [infra_test.go]"]
    e__repos_pith_pkg_parser_infra_test_go_testtestparser["[FUNCTION] testtestparser [infra_test.go]"]
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser["[FUNCTION] testgithubparser [infra_test.go]"]
    e__repos_pith_pkg_parser_interface_go["[MODULE] /repos/pith/pkg/parser/interface.go [interface.go]"]
    e__repos_pith_pkg_parser_interface_go_parser["[INTERFACE] parser [interface.go]"]
    e__repos_pith_pkg_parser_interface_go_matchcommand["[FUNCTION] matchcommand [interface.go]"]
    e__repos_pith_pkg_parser_interface_go_getallparsers["[FUNCTION] getallparsers [interface.go]"]
    e__repos_pith_pkg_parser_match_test_go["[MODULE] /repos/pith/pkg/parser/match_test.go [match_test.go]"]
    e__repos_pith_pkg_parser_match_test_go_testmatchcommand["[FUNCTION] testmatchcommand [match_test.go]"]
    e__repos_pith_pkg_parser_new_parsers_test_go["[MODULE] /repos/pith/pkg/parser/new_parsers_test.go [new_parsers_test.go]"]
    e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser["[FUNCTION] testsourceparser [new_parsers_test.go]"]
    e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser["[FUNCTION] testgithubreleaseparser [new_parsers_test.go]"]
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser["[FUNCTION] testchainparser [new_parsers_test.go]"]
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser["[FUNCTION] testwebparser [new_parsers_test.go]"]
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser["[FUNCTION] testpithparser [new_parsers_test.go]"]
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser["[FUNCTION] testgoparser [new_parsers_test.go]"]
    e__repos_pith_pkg_parser_npm_go["[MODULE] /repos/pith/pkg/parser/npm.go [npm.go]"]
    e__repos_pith_pkg_parser_npm_go_canparse["[FUNCTION] canparse [npm.go]"]
    e__repos_pith_pkg_parser_npm_go_parse["[FUNCTION] parse [npm.go]"]
    e__repos_pith_pkg_parser_npm_go_npmparser["[STRUCT] npmparser [npm.go]"]
    e__repos_pith_pkg_parser_npm_go_name["[FUNCTION] name [npm.go]"]
    e__repos_pith_pkg_parser_pith_go["[MODULE] /repos/pith/pkg/parser/pith.go [pith.go]"]
    e__repos_pith_pkg_parser_pith_go_name["[FUNCTION] name [pith.go]"]
    e__repos_pith_pkg_parser_pith_go_canparse["[FUNCTION] canparse [pith.go]"]
    e__repos_pith_pkg_parser_pith_go_parse["[FUNCTION] parse [pith.go]"]
    e__repos_pith_pkg_parser_pith_go_pithparser["[STRUCT] pithparser [pith.go]"]
    e__repos_pith_pkg_parser_powershell_go["[MODULE] /repos/pith/pkg/parser/powershell.go [powershell.go]"]
    e__repos_pith_pkg_parser_powershell_go_powershellparser["[STRUCT] powershellparser [powershell.go]"]
    e__repos_pith_pkg_parser_powershell_go_name["[FUNCTION] name [powershell.go]"]
    e__repos_pith_pkg_parser_powershell_go_canparse["[FUNCTION] canparse [powershell.go]"]
    e__repos_pith_pkg_parser_powershell_go_parse["[FUNCTION] parse [powershell.go]"]
    e__repos_pith_pkg_parser_powershell_go_getcontentparser["[STRUCT] getcontentparser [powershell.go]"]
    e__repos_pith_pkg_parser_promptfoo_go["[MODULE] /repos/pith/pkg/parser/promptfoo.go [promptfoo.go]"]
    e__repos_pith_pkg_parser_promptfoo_go_canparse["[FUNCTION] canparse [promptfoo.go]"]
    e__repos_pith_pkg_parser_promptfoo_go_parse["[FUNCTION] parse [promptfoo.go]"]
    e__repos_pith_pkg_parser_promptfoo_go_promptfooparser["[STRUCT] promptfooparser [promptfoo.go]"]
    e__repos_pith_pkg_parser_promptfoo_go_name["[FUNCTION] name [promptfoo.go]"]
    e__repos_pith_pkg_parser_source_go["[MODULE] /repos/pith/pkg/parser/source.go [source.go]"]
    e__repos_pith_pkg_parser_source_go_sourceparser["[STRUCT] sourceparser [source.go]"]
    e__repos_pith_pkg_parser_source_go_name["[FUNCTION] name [source.go]"]
    e__repos_pith_pkg_parser_source_go_canparse["[FUNCTION] canparse [source.go]"]
    e__repos_pith_pkg_parser_source_go_parse["[FUNCTION] parse [source.go]"]
    e__repos_pith_pkg_parser_text_go["[MODULE] /repos/pith/pkg/parser/text.go [text.go]"]
    e__repos_pith_pkg_parser_text_go_minifyparser["[STRUCT] minifyparser [text.go]"]
    e__repos_pith_pkg_parser_text_go_grepparser["[STRUCT] grepparser [text.go]"]
    e__repos_pith_pkg_parser_text_go_name["[FUNCTION] name [text.go]"]
    e__repos_pith_pkg_parser_text_go_canparse["[FUNCTION] canparse [text.go]"]
    e__repos_pith_pkg_parser_text_go_parse["[FUNCTION] parse [text.go]"]
    e__repos_pith_pkg_parser_text_test_go["[MODULE] /repos/pith/pkg/parser/text_test.go [text_test.go]"]
    e__repos_pith_pkg_parser_text_test_go_testgrepparser["[FUNCTION] testgrepparser [text_test.go]"]
    e__repos_pith_pkg_parser_text_test_go_testminifyparser["[FUNCTION] testminifyparser [text_test.go]"]
    e__repos_pith_pkg_parser_thneed_go["[MODULE] /repos/pith/pkg/parser/thneed.go [thneed.go]"]
    e__repos_pith_pkg_parser_thneed_go_parseplain["[FUNCTION] parseplain [thneed.go]"]
    e__repos_pith_pkg_parser_thneed_go_thneedparser["[STRUCT] thneedparser [thneed.go]"]
    e__repos_pith_pkg_parser_thneed_go_name["[FUNCTION] name [thneed.go]"]
    e__repos_pith_pkg_parser_thneed_go_canparse["[FUNCTION] canparse [thneed.go]"]
    e__repos_pith_pkg_parser_thneed_go_parse["[FUNCTION] parse [thneed.go]"]
    e__repos_pith_pkg_parser_thneed_go_parsejson["[FUNCTION] parsejson [thneed.go]"]
    e__repos_pith_pkg_parser_thneed_go_parsejsonobject["[FUNCTION] parsejsonobject [thneed.go]"]
    e__repos_pith_pkg_parser_vitest_go["[MODULE] /repos/pith/pkg/parser/vitest.go [vitest.go]"]
    e__repos_pith_pkg_parser_vitest_go_canparse["[FUNCTION] canparse [vitest.go]"]
    e__repos_pith_pkg_parser_vitest_go_parse["[FUNCTION] parse [vitest.go]"]
    e__repos_pith_pkg_parser_vitest_go_vitestparser["[STRUCT] vitestparser [vitest.go]"]
    e__repos_pith_pkg_parser_vitest_go_name["[FUNCTION] name [vitest.go]"]
    e__repos_pith_pkg_parser_vitest_test_go["[MODULE] /repos/pith/pkg/parser/vitest_test.go [vitest_test.go]"]
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser["[FUNCTION] testpromptfooparser [vitest_test.go]"]
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser["[FUNCTION] testpowershellparser [vitest_test.go]"]
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser["[FUNCTION] testvitestparser [vitest_test.go]"]
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser["[FUNCTION] testbdparser [vitest_test.go]"]
    e__repos_pith_pkg_parser_web_go["[MODULE] /repos/pith/pkg/parser/web.go [web.go]"]
    e__repos_pith_pkg_parser_web_go_webparser["[STRUCT] webparser [web.go]"]
    e__repos_pith_pkg_parser_web_go_name["[FUNCTION] name [web.go]"]
    e__repos_pith_pkg_parser_web_go_canparse["[FUNCTION] canparse [web.go]"]
    e__repos_pith_pkg_parser_web_go_parse["[FUNCTION] parse [web.go]"]
    fmt[["[EXTERNAL] fmt"]]
    e__repos_pith_pkg_parser_bd_go -.->|imports|  fmt
    strings[["[EXTERNAL] strings"]]
    e__repos_pith_pkg_parser_bd_go -.->|imports|  strings
    matchcommand[["[EXTERNAL] matchcommand"]]
    e__repos_pith_pkg_parser_bd_go_canparse -->|calls| matchcommand
    split[["[EXTERNAL] split"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| split
    strings_split[["[EXTERNAL] strings.split"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| strings_split
    trimspace[["[EXTERNAL] trimspace"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| trimspace
    strings_trimspace[["[EXTERNAL] strings.trimspace"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| strings_trimspace
    hasprefix[["[EXTERNAL] hasprefix"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| hasprefix
    strings_hasprefix[["[EXTERNAL] strings.hasprefix"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| strings_hasprefix
    contains[["[EXTERNAL] contains"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| contains
    strings_contains[["[EXTERNAL] strings.contains"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| strings_contains
    append[["[EXTERNAL] append"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| append
    len[["[EXTERNAL] len"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| len
    join[["[EXTERNAL] join"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| join
    strings_join[["[EXTERNAL] strings.join"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| strings_join
    sprintf[["[EXTERNAL] sprintf"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| sprintf
    fmt_sprintf[["[EXTERNAL] fmt.sprintf"]]
    e__repos_pith_pkg_parser_bd_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_bd_go ==>|contains| e__repos_pith_pkg_parser_bd_go_bdparser
    e__repos_pith_pkg_parser_bd_go ==>|contains| e__repos_pith_pkg_parser_bd_go_name
    e__repos_pith_pkg_parser_bd_go ==>|contains| e__repos_pith_pkg_parser_bd_go_canparse
    e__repos_pith_pkg_parser_bd_go ==>|contains| e__repos_pith_pkg_parser_bd_go_parse
    e__repos_pith_pkg_parser_chain_go -.->|imports|  strings
    e__repos_pith_pkg_parser_chain_go_canparse -->|calls| join
    e__repos_pith_pkg_parser_chain_go_canparse -->|calls| strings_join
    e__repos_pith_pkg_parser_chain_go_canparse -->|calls| append
    e__repos_pith_pkg_parser_chain_go_canparse -->|calls| contains
    e__repos_pith_pkg_parser_chain_go_canparse -->|calls| strings_contains
    e__repos_pith_pkg_parser_chain_go_splitsubcommands -->|calls| split
    e__repos_pith_pkg_parser_chain_go_splitsubcommands -->|calls| strings_split
    e__repos_pith_pkg_parser_chain_go_splitsubcommands -->|calls| trimspace
    e__repos_pith_pkg_parser_chain_go_splitsubcommands -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_chain_go_splitsubcommands -->|calls| append
    e__repos_pith_pkg_parser_chain_go ==>|contains| e__repos_pith_pkg_parser_chain_go_splitsubcommands
    e__repos_pith_pkg_parser_chain_go ==>|contains| e__repos_pith_pkg_parser_chain_go_chainparser
    e__repos_pith_pkg_parser_chain_go ==>|contains| e__repos_pith_pkg_parser_chain_go_name
    e__repos_pith_pkg_parser_chain_go ==>|contains| e__repos_pith_pkg_parser_chain_go_canparse
    e__repos_pith_pkg_parser_chain_go ==>|contains| e__repos_pith_pkg_parser_chain_go_parse
    e__repos_pith_pkg_parser_fs_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_fs_go -.->|imports|  strings
    e__repos_pith_pkg_parser_fs_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| split
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| strings_hasprefix
    fields[["[EXTERNAL] fields"]]
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| fields
    strings_fields[["[EXTERNAL] strings.fields"]]
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| strings_fields
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| len
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| append
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| join
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| strings_join
    newreplacer[["[EXTERNAL] newreplacer"]]
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| newreplacer
    strings_newreplacer[["[EXTERNAL] strings.newreplacer"]]
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| strings_newreplacer
    replace[["[EXTERNAL] replace"]]
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| replace
    r_replace[["[EXTERNAL] r.replace"]]
    e__repos_pith_pkg_parser_fs_go_parse -->|calls| r_replace
    e__repos_pith_pkg_parser_fs_go ==>|contains| e__repos_pith_pkg_parser_fs_go_lsparser
    e__repos_pith_pkg_parser_fs_go ==>|contains| e__repos_pith_pkg_parser_fs_go_name
    e__repos_pith_pkg_parser_fs_go ==>|contains| e__repos_pith_pkg_parser_fs_go_canparse
    e__repos_pith_pkg_parser_fs_go ==>|contains| e__repos_pith_pkg_parser_fs_go_parse
    e__repos_pith_pkg_parser_fs_go ==>|contains| e__repos_pith_pkg_parser_fs_go_findparser
    e__repos_pith_pkg_parser_fs_go ==>|contains| e__repos_pith_pkg_parser_fs_go_treeparser
    e__repos_pith_pkg_parser_fs_go ==>|contains| e__repos_pith_pkg_parser_fs_go_duparser
    e__repos_pith_pkg_parser_fs_test_go -.->|imports|  strings
    testing[["[EXTERNAL] testing"]]
    e__repos_pith_pkg_parser_fs_test_go -.->|imports|  testing
    canparse[["[EXTERNAL] canparse"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| canparse
    p_canparse[["[EXTERNAL] p.canparse"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| p_canparse
    error[["[EXTERNAL] error"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| error
    t_error[["[EXTERNAL] t.error"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| t_error
    parse[["[EXTERNAL] parse"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| parse
    p_parse[["[EXTERNAL] p.parse"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| p_parse
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| contains
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| strings_contains
    errorf[["[EXTERNAL] errorf"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| errorf
    t_errorf[["[EXTERNAL] t.errorf"]]
    e__repos_pith_pkg_parser_fs_test_go_testlsparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| canparse
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| error
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| t_error
    repeat[["[EXTERNAL] repeat"]]
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| repeat
    strings_repeat[["[EXTERNAL] strings.repeat"]]
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| strings_repeat
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| parse
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| p_parse
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| contains
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| errorf
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| len
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| split
    e__repos_pith_pkg_parser_fs_test_go_testfindparser -->|calls| strings_split
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| canparse
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| error
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| t_error
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| parse
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| p_parse
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| contains
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| errorf
    e__repos_pith_pkg_parser_fs_test_go_testtreeparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| canparse
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| error
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| t_error
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| parse
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| p_parse
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| contains
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| errorf
    e__repos_pith_pkg_parser_fs_test_go_testduparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_fs_test_go ==>|contains| e__repos_pith_pkg_parser_fs_test_go_testtreeparser
    e__repos_pith_pkg_parser_fs_test_go ==>|contains| e__repos_pith_pkg_parser_fs_test_go_testduparser
    e__repos_pith_pkg_parser_fs_test_go ==>|contains| e__repos_pith_pkg_parser_fs_test_go_testlsparser
    e__repos_pith_pkg_parser_fs_test_go ==>|contains| e__repos_pith_pkg_parser_fs_test_go_testfindparser
    e__repos_pith_pkg_parser_get_content_test_go -.->|imports|  strings
    e__repos_pith_pkg_parser_get_content_test_go -.->|imports|  testing
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| canparse
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| errorf
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| parse
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| p_parse
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| contains
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| repeat
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| strings_repeat
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| len
    hassuffix[["[EXTERNAL] hassuffix"]]
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| hassuffix
    strings_hassuffix[["[EXTERNAL] strings.hassuffix"]]
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| strings_hassuffix
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| split
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| strings_split
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| trimspace
    e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_get_content_test_go ==>|contains| e__repos_pith_pkg_parser_get_content_test_go_testgetcontentparser
    e__repos_pith_pkg_parser_git_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_git_go -.->|imports|  strings
    e__repos_pith_pkg_parser_git_go_canparse -->|calls| len
    e__repos_pith_pkg_parser_git_go_parse -->|calls| split
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_git_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_git_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_git_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_git_go_parse -->|calls| append
    e__repos_pith_pkg_parser_git_go_parse -->|calls| len
    e__repos_pith_pkg_parser_git_go_parse -->|calls| join
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_join
    formatcommit[["[EXTERNAL] formatcommit"]]
    e__repos_pith_pkg_parser_git_go_parse -->|calls| formatcommit
    trimprefix[["[EXTERNAL] trimprefix"]]
    e__repos_pith_pkg_parser_git_go_parse -->|calls| trimprefix
    strings_trimprefix[["[EXTERNAL] strings.trimprefix"]]
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_trimprefix
    index[["[EXTERNAL] index"]]
    e__repos_pith_pkg_parser_git_go_parse -->|calls| index
    strings_index[["[EXTERNAL] strings.index"]]
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_index
    e__repos_pith_pkg_parser_git_go_parse -->|calls| fields
    e__repos_pith_pkg_parser_git_go_parse -->|calls| strings_fields
    e__repos_pith_pkg_parser_git_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_git_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_git_go_canparse -->|calls| contains
    e__repos_pith_pkg_parser_git_go_canparse -->|calls| strings_contains
    make[["[EXTERNAL] make"]]
    e__repos_pith_pkg_parser_git_go_parse -->|calls| make
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_name
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_canparse
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_gitlogparser
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_formatcommit
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_gitdiffparser
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_gitbranchparser
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_compositegitparser
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_gitstatusparser
    e__repos_pith_pkg_parser_git_go ==>|contains| e__repos_pith_pkg_parser_git_go_parse
    e__repos_pith_pkg_parser_git_test_go -.->|imports|  strings
    e__repos_pith_pkg_parser_git_test_go -.->|imports|  testing
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| parse
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| p_parse
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| contains
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| errorf
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| error
    e__repos_pith_pkg_parser_git_test_go_testgitstatusparser -->|calls| t_error
    e__repos_pith_pkg_parser_git_test_go_testgitlogparser -->|calls| parse
    e__repos_pith_pkg_parser_git_test_go_testgitlogparser -->|calls| p_parse
    e__repos_pith_pkg_parser_git_test_go_testgitlogparser -->|calls| contains
    e__repos_pith_pkg_parser_git_test_go_testgitlogparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_git_test_go_testgitlogparser -->|calls| errorf
    e__repos_pith_pkg_parser_git_test_go_testgitlogparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_git_test_go_testgitdiffparser -->|calls| parse
    e__repos_pith_pkg_parser_git_test_go_testgitdiffparser -->|calls| p_parse
    e__repos_pith_pkg_parser_git_test_go_testgitdiffparser -->|calls| contains
    e__repos_pith_pkg_parser_git_test_go_testgitdiffparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_git_test_go_testgitdiffparser -->|calls| error
    e__repos_pith_pkg_parser_git_test_go_testgitdiffparser -->|calls| t_error
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| parse
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| p_parse
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| hasprefix
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| errorf
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| contains
    e__repos_pith_pkg_parser_git_test_go_testgitbranchparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| canparse
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| error
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| t_error
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| parse
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| p_parse
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| contains
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| errorf
    e__repos_pith_pkg_parser_git_test_go_testcompositegitparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_git_test_go ==>|contains| e__repos_pith_pkg_parser_git_test_go_testgitstatusparser
    e__repos_pith_pkg_parser_git_test_go ==>|contains| e__repos_pith_pkg_parser_git_test_go_testgitlogparser
    e__repos_pith_pkg_parser_git_test_go ==>|contains| e__repos_pith_pkg_parser_git_test_go_testgitdiffparser
    e__repos_pith_pkg_parser_git_test_go ==>|contains| e__repos_pith_pkg_parser_git_test_go_testgitbranchparser
    e__repos_pith_pkg_parser_git_test_go ==>|contains| e__repos_pith_pkg_parser_git_test_go_testcompositegitparser
    e__repos_pith_pkg_parser_github_release_go -.->|imports|  fmt
    regexp[["[EXTERNAL] regexp"]]
    e__repos_pith_pkg_parser_github_release_go -.->|imports|  regexp
    e__repos_pith_pkg_parser_github_release_go -.->|imports|  strings
    e__repos_pith_pkg_parser_github_release_go_canparse -->|calls| len
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| split
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| strings_split
    mustcompile[["[EXTERNAL] mustcompile"]]
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| mustcompile
    regexp_mustcompile[["[EXTERNAL] regexp.mustcompile"]]
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| regexp_mustcompile
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| append
    findstringsubmatch[["[EXTERNAL] findstringsubmatch"]]
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| findstringsubmatch
    reasset_findstringsubmatch[["[EXTERNAL] reasset.findstringsubmatch"]]
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| reasset_findstringsubmatch
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| len
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| fmt_sprintf
    replaceallstring[["[EXTERNAL] replaceallstring"]]
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| replaceallstring
    resha_replaceallstring[["[EXTERNAL] resha.replaceallstring"]]
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| resha_replaceallstring
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| join
    e__repos_pith_pkg_parser_github_release_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_github_release_go ==>|contains| e__repos_pith_pkg_parser_github_release_go_githubreleaseparser
    e__repos_pith_pkg_parser_github_release_go ==>|contains| e__repos_pith_pkg_parser_github_release_go_name
    e__repos_pith_pkg_parser_github_release_go ==>|contains| e__repos_pith_pkg_parser_github_release_go_canparse
    e__repos_pith_pkg_parser_github_release_go ==>|contains| e__repos_pith_pkg_parser_github_release_go_parse
    e__repos_pith_pkg_parser_go_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_go_go -.->|imports|  strings
    e__repos_pith_pkg_parser_go_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_go_go_parse -->|calls| split
    e__repos_pith_pkg_parser_go_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_go_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_go_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_go_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_go_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_go_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_go_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_go_go_parse -->|calls| append
    e__repos_pith_pkg_parser_go_go_parse -->|calls| len
    e__repos_pith_pkg_parser_go_go_parse -->|calls| fields
    e__repos_pith_pkg_parser_go_go_parse -->|calls| strings_fields
    e__repos_pith_pkg_parser_go_go_parse -->|calls| join
    e__repos_pith_pkg_parser_go_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_go_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_go_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_go_go ==>|contains| e__repos_pith_pkg_parser_go_go_name
    e__repos_pith_pkg_parser_go_go ==>|contains| e__repos_pith_pkg_parser_go_go_canparse
    e__repos_pith_pkg_parser_go_go ==>|contains| e__repos_pith_pkg_parser_go_go_parse
    e__repos_pith_pkg_parser_go_go ==>|contains| e__repos_pith_pkg_parser_go_go_goparser
    e__repos_pith_pkg_parser_infra_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_infra_go -.->|imports|  regexp
    e__repos_pith_pkg_parser_infra_go -.->|imports|  strings
    e__repos_pith_pkg_parser_infra_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_infra_go -->|calls| mustcompile
    e__repos_pith_pkg_parser_infra_go -->|calls| regexp_mustcompile
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| split
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_contains
    splitn[["[EXTERNAL] splitn"]]
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| splitn
    strings_splitn[["[EXTERNAL] strings.splitn"]]
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_splitn
    matchstring[["[EXTERNAL] matchstring"]]
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| matchstring
    skipenvregex_matchstring[["[EXTERNAL] skipenvregex.matchstring"]]
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| skipenvregex_matchstring
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| len
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| append
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| join
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_infra_go_canparse -->|calls| len
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| fields
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_fields
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| newreplacer
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_newreplacer
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| replace
    replacer_replace[["[EXTERNAL] replacer.replace"]]
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| replacer_replace
    map[["[EXTERNAL] map"]]
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| map
    strings_map[["[EXTERNAL] strings.map"]]
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_map
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_infra_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_infra_go_canparse -->|calls| contains
    e__repos_pith_pkg_parser_infra_go_canparse -->|calls| strings_contains
    e__repos_pith_pkg_parser_infra_go_canparse -->|calls| join
    e__repos_pith_pkg_parser_infra_go_canparse -->|calls| strings_join
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_parse
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_dockerpsparser
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_dependencyparser
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_testparser
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_githubparser
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_envparser
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_name
    e__repos_pith_pkg_parser_infra_go ==>|contains| e__repos_pith_pkg_parser_infra_go_canparse
    e__repos_pith_pkg_parser_infra_test_go -.->|imports|  strings
    e__repos_pith_pkg_parser_infra_test_go -.->|imports|  testing
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| canparse
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| error
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| t_error
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| parse
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| p_parse
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| contains
    e__repos_pith_pkg_parser_infra_test_go_testenvparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| canparse
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| error
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| t_error
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| parse
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| p_parse
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| contains
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| errorf
    e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| canparse
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| error
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| t_error
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| parse
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| p_parse
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| contains
    e__repos_pith_pkg_parser_infra_test_go_testdependencyparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| canparse
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| error
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| t_error
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| parse
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| p_parse
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| contains
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| errorf
    e__repos_pith_pkg_parser_infra_test_go_testtestparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| canparse
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| error
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| t_error
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| parse
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| p_parse
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| contains
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| errorf
    e__repos_pith_pkg_parser_infra_test_go_testgithubparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_infra_test_go ==>|contains| e__repos_pith_pkg_parser_infra_test_go_testenvparser
    e__repos_pith_pkg_parser_infra_test_go ==>|contains| e__repos_pith_pkg_parser_infra_test_go_testdockerpsparser
    e__repos_pith_pkg_parser_infra_test_go ==>|contains| e__repos_pith_pkg_parser_infra_test_go_testdependencyparser
    e__repos_pith_pkg_parser_infra_test_go ==>|contains| e__repos_pith_pkg_parser_infra_test_go_testtestparser
    e__repos_pith_pkg_parser_infra_test_go ==>|contains| e__repos_pith_pkg_parser_infra_test_go_testgithubparser
    path_filepath[["[EXTERNAL] filepath"]]
    e__repos_pith_pkg_parser_interface_go -.->|imports|  path_filepath
    e__repos_pith_pkg_parser_interface_go -.->|imports|  strings
    replaceall[["[EXTERNAL] replaceall"]]
    e__repos_pith_pkg_parser_interface_go_matchcommand -->|calls| replaceall
    strings_replaceall[["[EXTERNAL] strings.replaceall"]]
    e__repos_pith_pkg_parser_interface_go_matchcommand -->|calls| strings_replaceall
    tolower[["[EXTERNAL] tolower"]]
    e__repos_pith_pkg_parser_interface_go_matchcommand -->|calls| tolower
    strings_tolower[["[EXTERNAL] strings.tolower"]]
    e__repos_pith_pkg_parser_interface_go_matchcommand -->|calls| strings_tolower
    base[["[EXTERNAL] base"]]
    e__repos_pith_pkg_parser_interface_go_matchcommand -->|calls| base
    filepath_base[["[EXTERNAL] filepath.base"]]
    e__repos_pith_pkg_parser_interface_go_matchcommand -->|calls| filepath_base
    e__repos_pith_pkg_parser_interface_go_getallparsers -->|calls| append
    e__repos_pith_pkg_parser_interface_go ==>|contains| e__repos_pith_pkg_parser_interface_go_parser
    e__repos_pith_pkg_parser_interface_go ==>|contains| e__repos_pith_pkg_parser_interface_go_matchcommand
    e__repos_pith_pkg_parser_interface_go ==>|contains| e__repos_pith_pkg_parser_interface_go_getallparsers
    e__repos_pith_pkg_parser_match_test_go -.->|imports|  testing
    run[["[EXTERNAL] run"]]
    e__repos_pith_pkg_parser_match_test_go_testmatchcommand -->|calls| run
    t_run[["[EXTERNAL] t.run"]]
    e__repos_pith_pkg_parser_match_test_go_testmatchcommand -->|calls| t_run
    e__repos_pith_pkg_parser_match_test_go_testmatchcommand -->|calls| matchcommand
    e__repos_pith_pkg_parser_match_test_go_testmatchcommand -->|calls| errorf
    e__repos_pith_pkg_parser_match_test_go_testmatchcommand -->|calls| t_errorf
    e__repos_pith_pkg_parser_match_test_go ==>|contains| e__repos_pith_pkg_parser_match_test_go_testmatchcommand
    e__repos_pith_pkg_parser_new_parsers_test_go -.->|imports|  testing
    e__repos_pith_pkg_parser_new_parsers_test_go -.->|imports|  strings
    e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser -->|calls| parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser -->|calls| p_parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser -->|calls| contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser -->|calls| errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser -->|calls| parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser -->|calls| p_parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser -->|calls| contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser -->|calls| errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser -->|calls| canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser -->|calls| errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser -->|calls| t_errorf
    splitsubcommands[["[EXTERNAL] splitsubcommands"]]
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser -->|calls| splitsubcommands
    p_splitsubcommands[["[EXTERNAL] p.splitsubcommands"]]
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser -->|calls| p_splitsubcommands
    e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser -->|calls| len
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| error
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| t_error
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| p_parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| error
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| t_error
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| p_parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| error
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| t_error
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| p_parse
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| errorf
    e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_new_parsers_test_go ==>|contains| e__repos_pith_pkg_parser_new_parsers_test_go_testsourceparser
    e__repos_pith_pkg_parser_new_parsers_test_go ==>|contains| e__repos_pith_pkg_parser_new_parsers_test_go_testgithubreleaseparser
    e__repos_pith_pkg_parser_new_parsers_test_go ==>|contains| e__repos_pith_pkg_parser_new_parsers_test_go_testchainparser
    e__repos_pith_pkg_parser_new_parsers_test_go ==>|contains| e__repos_pith_pkg_parser_new_parsers_test_go_testwebparser
    e__repos_pith_pkg_parser_new_parsers_test_go ==>|contains| e__repos_pith_pkg_parser_new_parsers_test_go_testpithparser
    e__repos_pith_pkg_parser_new_parsers_test_go ==>|contains| e__repos_pith_pkg_parser_new_parsers_test_go_testgoparser
    e__repos_pith_pkg_parser_npm_go -.->|imports|  strings
    e__repos_pith_pkg_parser_npm_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| split
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| append
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| len
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| join
    e__repos_pith_pkg_parser_npm_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_npm_go ==>|contains| e__repos_pith_pkg_parser_npm_go_canparse
    e__repos_pith_pkg_parser_npm_go ==>|contains| e__repos_pith_pkg_parser_npm_go_parse
    e__repos_pith_pkg_parser_npm_go ==>|contains| e__repos_pith_pkg_parser_npm_go_npmparser
    e__repos_pith_pkg_parser_npm_go ==>|contains| e__repos_pith_pkg_parser_npm_go_name
    e__repos_pith_pkg_parser_pith_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_pith_go -.->|imports|  strings
    e__repos_pith_pkg_parser_pith_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_pith_go_canparse -->|calls| len
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| split
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| append
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| len
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| join
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_pith_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_pith_go ==>|contains| e__repos_pith_pkg_parser_pith_go_name
    e__repos_pith_pkg_parser_pith_go ==>|contains| e__repos_pith_pkg_parser_pith_go_canparse
    e__repos_pith_pkg_parser_pith_go ==>|contains| e__repos_pith_pkg_parser_pith_go_parse
    e__repos_pith_pkg_parser_pith_go ==>|contains| e__repos_pith_pkg_parser_pith_go_pithparser
    e__repos_pith_pkg_parser_powershell_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_powershell_go -.->|imports|  strings
    e__repos_pith_pkg_parser_powershell_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| split
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| fields
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| strings_fields
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| len
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| append
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| join
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_powershell_go_canparse -->|calls| join
    e__repos_pith_pkg_parser_powershell_go_canparse -->|calls| strings_join
    e__repos_pith_pkg_parser_powershell_go_canparse -->|calls| append
    e__repos_pith_pkg_parser_powershell_go_canparse -->|calls| contains
    e__repos_pith_pkg_parser_powershell_go_canparse -->|calls| strings_contains
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| hassuffix
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| strings_hassuffix
    writebyte[["[EXTERNAL] writebyte"]]
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| writebyte
    minified_writebyte[["[EXTERNAL] minified.writebyte"]]
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| minified_writebyte
    string[["[EXTERNAL] string"]]
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| string
    minified_string[["[EXTERNAL] minified.string"]]
    e__repos_pith_pkg_parser_powershell_go_parse -->|calls| minified_string
    e__repos_pith_pkg_parser_powershell_go ==>|contains| e__repos_pith_pkg_parser_powershell_go_powershellparser
    e__repos_pith_pkg_parser_powershell_go ==>|contains| e__repos_pith_pkg_parser_powershell_go_name
    e__repos_pith_pkg_parser_powershell_go ==>|contains| e__repos_pith_pkg_parser_powershell_go_canparse
    e__repos_pith_pkg_parser_powershell_go ==>|contains| e__repos_pith_pkg_parser_powershell_go_parse
    e__repos_pith_pkg_parser_powershell_go ==>|contains| e__repos_pith_pkg_parser_powershell_go_getcontentparser
    e__repos_pith_pkg_parser_promptfoo_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_promptfoo_go -.->|imports|  strings
    e__repos_pith_pkg_parser_promptfoo_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_promptfoo_go_canparse -->|calls| len
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| split
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| append
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| len
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| join
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_promptfoo_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_promptfoo_go ==>|contains| e__repos_pith_pkg_parser_promptfoo_go_canparse
    e__repos_pith_pkg_parser_promptfoo_go ==>|contains| e__repos_pith_pkg_parser_promptfoo_go_parse
    e__repos_pith_pkg_parser_promptfoo_go ==>|contains| e__repos_pith_pkg_parser_promptfoo_go_promptfooparser
    e__repos_pith_pkg_parser_promptfoo_go ==>|contains| e__repos_pith_pkg_parser_promptfoo_go_name
    e__repos_pith_pkg_parser_source_go -.->|imports|  regexp
    e__repos_pith_pkg_parser_source_go -.->|imports|  strings
    e__repos_pith_pkg_parser_source_go_parse -->|calls| split
    e__repos_pith_pkg_parser_source_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_source_go_parse -->|calls| mustcompile
    e__repos_pith_pkg_parser_source_go_parse -->|calls| regexp_mustcompile
    e__repos_pith_pkg_parser_source_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_source_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_source_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_source_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_source_go_parse -->|calls| replaceallstring
    reinline_replaceallstring[["[EXTERNAL] reinline.replaceallstring"]]
    e__repos_pith_pkg_parser_source_go_parse -->|calls| reinline_replaceallstring
    respaces_replaceallstring[["[EXTERNAL] respaces.replaceallstring"]]
    e__repos_pith_pkg_parser_source_go_parse -->|calls| respaces_replaceallstring
    e__repos_pith_pkg_parser_source_go_parse -->|calls| append
    e__repos_pith_pkg_parser_source_go_parse -->|calls| join
    e__repos_pith_pkg_parser_source_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_source_go ==>|contains| e__repos_pith_pkg_parser_source_go_sourceparser
    e__repos_pith_pkg_parser_source_go ==>|contains| e__repos_pith_pkg_parser_source_go_name
    e__repos_pith_pkg_parser_source_go ==>|contains| e__repos_pith_pkg_parser_source_go_canparse
    e__repos_pith_pkg_parser_source_go ==>|contains| e__repos_pith_pkg_parser_source_go_parse
    e__repos_pith_pkg_parser_text_go -.->|imports|  regexp
    e__repos_pith_pkg_parser_text_go -.->|imports|  strings
    e__repos_pith_pkg_parser_text_go_parse -->|calls| split
    e__repos_pith_pkg_parser_text_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_text_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_text_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_text_go_parse -->|calls| splitn
    e__repos_pith_pkg_parser_text_go_parse -->|calls| strings_splitn
    e__repos_pith_pkg_parser_text_go_parse -->|calls| len
    e__repos_pith_pkg_parser_text_go_parse -->|calls| append
    e__repos_pith_pkg_parser_text_go_parse -->|calls| join
    e__repos_pith_pkg_parser_text_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_text_go_canparse -->|calls| len
    e__repos_pith_pkg_parser_text_go_canparse -->|calls| hassuffix
    e__repos_pith_pkg_parser_text_go_canparse -->|calls| strings_hassuffix
    e__repos_pith_pkg_parser_text_go -->|calls| mustcompile
    e__repos_pith_pkg_parser_text_go -->|calls| regexp_mustcompile
    e__repos_pith_pkg_parser_text_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_text_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_text_go_parse -->|calls| replaceallstring
    whitespaceregex_replaceallstring[["[EXTERNAL] whitespaceregex.replaceallstring"]]
    e__repos_pith_pkg_parser_text_go_parse -->|calls| whitespaceregex_replaceallstring
    e__repos_pith_pkg_parser_text_go ==>|contains| e__repos_pith_pkg_parser_text_go_minifyparser
    e__repos_pith_pkg_parser_text_go ==>|contains| e__repos_pith_pkg_parser_text_go_grepparser
    e__repos_pith_pkg_parser_text_go ==>|contains| e__repos_pith_pkg_parser_text_go_name
    e__repos_pith_pkg_parser_text_go ==>|contains| e__repos_pith_pkg_parser_text_go_canparse
    e__repos_pith_pkg_parser_text_go ==>|contains| e__repos_pith_pkg_parser_text_go_parse
    e__repos_pith_pkg_parser_text_test_go -.->|imports|  strings
    e__repos_pith_pkg_parser_text_test_go -.->|imports|  testing
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| parse
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| p_parse
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| contains
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| errorf
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| t_errorf
    count[["[EXTERNAL] count"]]
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| count
    strings_count[["[EXTERNAL] strings.count"]]
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| strings_count
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| error
    e__repos_pith_pkg_parser_text_test_go_testgrepparser -->|calls| t_error
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| canparse
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| error
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| t_error
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| parse
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| p_parse
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| contains
    e__repos_pith_pkg_parser_text_test_go_testminifyparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_text_test_go ==>|contains| e__repos_pith_pkg_parser_text_test_go_testgrepparser
    e__repos_pith_pkg_parser_text_test_go ==>|contains| e__repos_pith_pkg_parser_text_test_go_testminifyparser
    encoding_json[["[EXTERNAL] json"]]
    e__repos_pith_pkg_parser_thneed_go -.->|imports|  encoding_json
    e__repos_pith_pkg_parser_thneed_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_thneed_go -.->|imports|  strings
    e__repos_pith_pkg_parser_thneed_go_canparse -->|calls| matchcommand
    unmarshal[["[EXTERNAL] unmarshal"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| unmarshal
    json_unmarshal[["[EXTERNAL] json.unmarshal"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| json_unmarshal
    parsejson[["[EXTERNAL] parsejson"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| parsejson
    t_parsejson[["[EXTERNAL] t.parsejson"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| t_parsejson
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| fmt_sprintf
    parsejsonobject[["[EXTERNAL] parsejsonobject"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| parsejsonobject
    t_parsejsonobject[["[EXTERNAL] t.parsejsonobject"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| t_parsejsonobject
    parseplain[["[EXTERNAL] parseplain"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| parseplain
    t_parseplain[["[EXTERNAL] t.parseplain"]]
    e__repos_pith_pkg_parser_thneed_go_parse -->|calls| t_parseplain
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| len
    writestring[["[EXTERNAL] writestring"]]
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| writestring
    sb_writestring[["[EXTERNAL] sb.writestring"]]
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| sb_writestring
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| sprintf
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| fmt_sprintf
    lastindex[["[EXTERNAL] lastindex"]]
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| lastindex
    strings_lastindex[["[EXTERNAL] strings.lastindex"]]
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| strings_lastindex
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| trimspace
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| replaceall
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| strings_replaceall
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| string
    sb_string[["[EXTERNAL] sb.string"]]
    e__repos_pith_pkg_parser_thneed_go_parsejson -->|calls| sb_string
    e__repos_pith_pkg_parser_thneed_go_parsejsonobject -->|calls| sprintf
    e__repos_pith_pkg_parser_thneed_go_parsejsonobject -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| split
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| strings_split
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| trimspace
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| hasprefix
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| append
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| len
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| join
    e__repos_pith_pkg_parser_thneed_go_parseplain -->|calls| strings_join
    e__repos_pith_pkg_parser_thneed_go ==>|contains| e__repos_pith_pkg_parser_thneed_go_parseplain
    e__repos_pith_pkg_parser_thneed_go ==>|contains| e__repos_pith_pkg_parser_thneed_go_thneedparser
    e__repos_pith_pkg_parser_thneed_go ==>|contains| e__repos_pith_pkg_parser_thneed_go_name
    e__repos_pith_pkg_parser_thneed_go ==>|contains| e__repos_pith_pkg_parser_thneed_go_canparse
    e__repos_pith_pkg_parser_thneed_go ==>|contains| e__repos_pith_pkg_parser_thneed_go_parse
    e__repos_pith_pkg_parser_thneed_go ==>|contains| e__repos_pith_pkg_parser_thneed_go_parsejson
    e__repos_pith_pkg_parser_thneed_go ==>|contains| e__repos_pith_pkg_parser_thneed_go_parsejsonobject
    e__repos_pith_pkg_parser_vitest_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_vitest_go -.->|imports|  strings
    e__repos_pith_pkg_parser_vitest_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_vitest_go_canparse -->|calls| len
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| split
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| strings_split
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| append
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| hasprefix
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| strings_hasprefix
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| len
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| join
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_vitest_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_vitest_go ==>|contains| e__repos_pith_pkg_parser_vitest_go_canparse
    e__repos_pith_pkg_parser_vitest_go ==>|contains| e__repos_pith_pkg_parser_vitest_go_parse
    e__repos_pith_pkg_parser_vitest_go ==>|contains| e__repos_pith_pkg_parser_vitest_go_vitestparser
    e__repos_pith_pkg_parser_vitest_go ==>|contains| e__repos_pith_pkg_parser_vitest_go_name
    e__repos_pith_pkg_parser_vitest_test_go -.->|imports|  strings
    e__repos_pith_pkg_parser_vitest_test_go -.->|imports|  testing
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| canparse
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| errorf
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| parse
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| p_parse
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| contains
    e__repos_pith_pkg_parser_vitest_test_go_testvitestparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| canparse
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| errorf
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| parse
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| p_parse
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| contains
    e__repos_pith_pkg_parser_vitest_test_go_testbdparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| canparse
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| errorf
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| parse
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| p_parse
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| contains
    e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| canparse
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| p_canparse
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| errorf
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| t_errorf
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| parse
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| p_parse
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| contains
    e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser -->|calls| strings_contains
    e__repos_pith_pkg_parser_vitest_test_go ==>|contains| e__repos_pith_pkg_parser_vitest_test_go_testpromptfooparser
    e__repos_pith_pkg_parser_vitest_test_go ==>|contains| e__repos_pith_pkg_parser_vitest_test_go_testpowershellparser
    e__repos_pith_pkg_parser_vitest_test_go ==>|contains| e__repos_pith_pkg_parser_vitest_test_go_testvitestparser
    e__repos_pith_pkg_parser_vitest_test_go ==>|contains| e__repos_pith_pkg_parser_vitest_test_go_testbdparser
    e__repos_pith_pkg_parser_web_go -.->|imports|  encoding_json
    e__repos_pith_pkg_parser_web_go -.->|imports|  fmt
    e__repos_pith_pkg_parser_web_go -.->|imports|  strings
    e__repos_pith_pkg_parser_web_go_canparse -->|calls| matchcommand
    e__repos_pith_pkg_parser_web_go_parse -->|calls| trimspace
    e__repos_pith_pkg_parser_web_go_parse -->|calls| strings_trimspace
    e__repos_pith_pkg_parser_web_go_parse -->|calls| unmarshal
    e__repos_pith_pkg_parser_web_go_parse -->|calls| json_unmarshal
    marshal[["[EXTERNAL] marshal"]]
    e__repos_pith_pkg_parser_web_go_parse -->|calls| marshal
    json_marshal[["[EXTERNAL] json.marshal"]]
    e__repos_pith_pkg_parser_web_go_parse -->|calls| json_marshal
    e__repos_pith_pkg_parser_web_go_parse -->|calls| len
    e__repos_pith_pkg_parser_web_go_parse -->|calls| make
    e__repos_pith_pkg_parser_web_go_parse -->|calls| append
    e__repos_pith_pkg_parser_web_go_parse -->|calls| sprintf
    e__repos_pith_pkg_parser_web_go_parse -->|calls| fmt_sprintf
    e__repos_pith_pkg_parser_web_go_parse -->|calls| join
    e__repos_pith_pkg_parser_web_go_parse -->|calls| strings_join
    e__repos_pith_pkg_parser_web_go_parse -->|calls| string
    e__repos_pith_pkg_parser_web_go_parse -->|calls| contains
    e__repos_pith_pkg_parser_web_go_parse -->|calls| strings_contains
    e__repos_pith_pkg_parser_web_go_parse -->|calls| index
    e__repos_pith_pkg_parser_web_go_parse -->|calls| strings_index
    e__repos_pith_pkg_parser_web_go ==>|contains| e__repos_pith_pkg_parser_web_go_webparser
    e__repos_pith_pkg_parser_web_go ==>|contains| e__repos_pith_pkg_parser_web_go_name
    e__repos_pith_pkg_parser_web_go ==>|contains| e__repos_pith_pkg_parser_web_go_canparse
    e__repos_pith_pkg_parser_web_go ==>|contains| e__repos_pith_pkg_parser_web_go_parse
```
<!-- mermaid-end -->
