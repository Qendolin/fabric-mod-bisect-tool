# Details

Date : 2025-07-01 18:09:45

Directory e:\\Documents\\Dev\\Shared\\fabric-mod-bisect-tool-2

Total : 58 files,  6905 codes, 992 comments, 1379 blanks, all 9276 lines

[Summary](results.md) / Details / [Diff Summary](diff.md) / [Diff Details](diff-details.md)

## Files
| filename | language | code | comment | blank | total |
| :--- | :--- | ---: | ---: | ---: | ---: |
| [.github/workflows/go.yml](/.github/workflows/go.yml) | YAML | 19 | 2 | 8 | 29 |
| [.github/workflows/release.yml](/.github/workflows/release.yml) | YAML | 27 | 2 | 4 | 33 |
| [README.md](/README.md) | Markdown | 28 | 0 | 10 | 38 |
| [app/app.go](/app/app.go) | Go | 332 | 27 | 57 | 416 |
| [app/config.go](/app/config.go) | Go | 17 | 2 | 6 | 25 |
| [app/core/bisect/enumeration.go](/app/core/bisect/enumeration.go) | Go | 52 | 11 | 13 | 76 |
| [app/core/bisect/service.go](/app/core/bisect/service.go) | Go | 197 | 68 | 56 | 321 |
| [app/core/imcs/engine.go](/app/core/imcs/engine.go) | Go | 169 | 57 | 45 | 271 |
| [app/core/imcs/errors.go](/app/core/imcs/errors.go) | Go | 7 | 0 | 3 | 10 |
| [app/core/imcs/execution\_log.go](/app/core/imcs/execution_log.go) | Go | 45 | 10 | 12 | 67 |
| [app/core/imcs/history.go](/app/core/imcs/history.go) | Go | 78 | 7 | 17 | 102 |
| [app/core/imcs/imcs\_algorithm.go](/app/core/imcs/imcs_algorithm.go) | Go | 85 | 32 | 23 | 140 |
| [app/core/imcs/state.go](/app/core/imcs/state.go) | Go | 77 | 42 | 20 | 139 |
| [app/core/integration\_test.go](/app/core/integration_test.go) | Go | 321 | 22 | 46 | 389 |
| [app/core/mods/activator.go](/app/core/mods/activator.go) | Go | 147 | 28 | 31 | 206 |
| [app/core/mods/loader.go](/app/core/mods/loader.go) | Go | 398 | 44 | 71 | 513 |
| [app/core/mods/manager.go](/app/core/mods/manager.go) | Go | 166 | 32 | 23 | 221 |
| [app/core/mods/overrides.go](/app/core/mods/overrides.go) | Go | 195 | 23 | 28 | 246 |
| [app/core/mods/parser.go](/app/core/mods/parser.go) | Go | 165 | 10 | 24 | 199 |
| [app/core/mods/resolver.go](/app/core/mods/resolver.go) | Go | 336 | 41 | 65 | 442 |
| [app/core/mods/types.go](/app/core/mods/types.go) | Go | 106 | 17 | 21 | 144 |
| [app/core/mods/version.go](/app/core/mods/version.go) | Go | 177 | 21 | 29 | 227 |
| [app/core/sets/ops.go](/app/core/sets/ops.go) | Go | 86 | 16 | 13 | 115 |
| [app/core/sets/types.go](/app/core/sets/types.go) | Go | 17 | 11 | 7 | 35 |
| [app/embeds/embeds.go](/app/embeds/embeds.go) | Go | 6 | 2 | 4 | 12 |
| [app/embeds/fabric\_loader\_dependencies.json](/app/embeds/fabric_loader_dependencies.json) | JSON | 12 | 0 | 1 | 13 |
| [app/logging/entry.go](/app/logging/entry.go) | Go | 28 | 4 | 6 | 38 |
| [app/logging/logger.go](/app/logging/logger.go) | Go | 138 | 29 | 35 | 202 |
| [app/logging/store.go](/app/logging/store.go) | Go | 23 | 5 | 6 | 34 |
| [app/report\_generator.go](/app/report_generator.go) | Go | 66 | 6 | 13 | 85 |
| [app/ui/dialogs.go](/app/ui/dialogs.go) | Go | 108 | 7 | 12 | 127 |
| [app/ui/focus\_manager.go](/app/ui/focus_manager.go) | Go | 123 | 9 | 29 | 161 |
| [app/ui/interfaces.go](/app/ui/interfaces.go) | Go | 51 | 14 | 8 | 73 |
| [app/ui/layout\_manager.go](/app/ui/layout_manager.go) | Go | 90 | 8 | 17 | 115 |
| [app/ui/navigation\_manager.go](/app/ui/navigation_manager.go) | Go | 114 | 20 | 25 | 159 |
| [app/ui/pages.go](/app/ui/pages.go) | Go | 24 | 5 | 6 | 35 |
| [app/ui/pages/history\_page.go](/app/ui/pages/history_page.go) | Go | 158 | 20 | 46 | 224 |
| [app/ui/pages/loading\_page.go](/app/ui/pages/loading_page.go) | Go | 83 | 11 | 20 | 114 |
| [app/ui/pages/log\_page.go](/app/ui/pages/log_page.go) | Go | 162 | 14 | 27 | 203 |
| [app/ui/pages/main\_page.go](/app/ui/pages/main_page.go) | Go | 250 | 31 | 46 | 327 |
| [app/ui/pages/manage\_mods\_page.go](/app/ui/pages/manage_mods_page.go) | Go | 361 | 34 | 59 | 454 |
| [app/ui/pages/result\_page.go](/app/ui/pages/result_page.go) | Go | 170 | 15 | 33 | 218 |
| [app/ui/pages/setup\_page.go](/app/ui/pages/setup_page.go) | Go | 129 | 5 | 24 | 158 |
| [app/ui/pages/test\_page.go](/app/ui/pages/test_page.go) | Go | 122 | 7 | 30 | 159 |
| [app/ui/widgets/flex\_list.go](/app/ui/widgets/flex_list.go) | Go | 153 | 27 | 29 | 209 |
| [app/ui/widgets/focus\_wrapper.go](/app/ui/widgets/focus_wrapper.go) | Go | 57 | 16 | 14 | 87 |
| [app/ui/widgets/overview\_widget.go](/app/ui/widgets/overview_widget.go) | Go | 136 | 23 | 28 | 187 |
| [app/ui/widgets/searchable\_list.go](/app/ui/widgets/searchable_list.go) | Go | 88 | 7 | 21 | 116 |
| [app/ui/widgets/searchable\_table.go](/app/ui/widgets/searchable_table.go) | Go | 187 | 25 | 37 | 249 |
| [app/ui/widgets/separators.go](/app/ui/widgets/separators.go) | Go | 41 | 6 | 8 | 55 |
| [app/ui/widgets/style.go](/app/ui/widgets/style.go) | Go | 15 | 0 | 4 | 19 |
| [app/ui/widgets/tabbed\_view.go](/app/ui/widgets/tabbed_view.go) | Go | 192 | 23 | 35 | 250 |
| [app/ui/widgets/title\_frame.go](/app/ui/widgets/title_frame.go) | Go | 106 | 19 | 22 | 147 |
| [docs/IterativeMinimalConflictSearch.md](/docs/IterativeMinimalConflictSearch.md) | Markdown | 76 | 0 | 18 | 94 |
| [docs/benchmark.py](/docs/benchmark.py) | Python | 229 | 67 | 66 | 362 |
| [go.mod](/go.mod) | Go Module File | 16 | 0 | 4 | 20 |
| [go.sum](/go.sum) | Go Checksum File | 88 | 0 | 1 | 89 |
| [main.go](/main.go) | Go | 56 | 8 | 13 | 77 |

[Summary](results.md) / Details / [Diff Summary](diff.md) / [Diff Details](diff-details.md)