---
sidebar_position: 3
---

# link

把一個資料夾（例如你自己的 skills checkout）直接連結到 skills source 底下。開啟 [`follow_source_links`](../targets/configuration.md#follow_source_links) 後，skillshare 會列出並 sync 其中的 skills。

```bash
skillshare link ~/code/dev-skills                  # 以 _dev-skills 連結
skillshare link ~/code/dev-skills --enable         # 連結並開啟跟進
skillshare link ~/code/dev-skills --name _mine     # 自訂連結名稱
skillshare link ../team --name _team -p            # 連結進專案的 source
skillshare link -- -checkout                       # 連結以 - 開頭的路徑
```

## 何時使用

- 在 skills checkout 原本的位置繼續編輯，而不是安裝一份複本
- 使用[外接硬碟](../targets/configuration.md#skills-on-an-external-drive)上的 skills
- 在 Windows 上建立連結，不必自己在 junction 與 symlink 之間做選擇

```text
$ skillshare link ~/code/dev-skills --enable
✓ Linked _dev-skills → ~/code/dev-skills · symlink
✓ Set follow_source_links: true ~/.config/skillshare/config.yaml

Next
  skillshare sync  link its skills into your targets
```

## 發生了什麼

1. 連結名稱預設為 `_` 加上資料夾名稱（`_dev-skills`），必須是直接位於 source 底下的單一名稱。
2. 以 discovery 檢查已跟進連結的相同方式檢查資料夾。若名稱已存在，或資料夾是 source 本身或其上層、位於 source 內、與 sync target 重疊、不存在、無法讀取或不是目錄，連結會被拒絕。
3. 建立連結：macOS 與 Linux 上是絕對路徑的 symlink。Windows 上會建立 junction，不需要開發人員模式或系統管理員權限；只有 junction 建立失敗時才改試目錄 symlink。
4. 資料夾沒有 `.git` 項目時會顯示警告：連結仍然可用，但 `skillshare update` 無法 pull 它。
5. `follow_source_links` 關閉時，除非加上 `--enable`，否則不會改動設定。沒有加時，指令會印出與 `skillshare doctor` 相同的提示。如果 `--enable` 無法儲存設定，新建的連結會被移除、指令失敗，不會留下未被跟隨的連結。Dashboard 的 **Link folder** 核取方塊也一樣。

## 選項

| 旗標 | 說明 |
|------|-------------|
| `--name <name>` | 在 source 中的連結名稱（預設：`_<資料夾名稱>`） |
| `--enable` | 同時設定 `follow_source_links: true` |
| `--project, -p` | 使用目前目錄的專案層級設定 |
| `--global, -g` | 使用全域設定（`~/.config/skillshare`） |
| `--` | 結束選項；允許以 `-` 開頭的路徑 |
| `--help, -h` | 顯示說明 |

## 不加 `--enable`

```text
$ skillshare link ~/code/dev-skills
✓ Linked _dev-skills → ~/code/dev-skills · symlink
! target is not a git checkout; skillshare update cannot pull it
  _dev-skills: not followed by discovery; its contents are invisible to skillshare. Set follow_source_links: true to follow it
```

連結已建立，但在設定中設好 `follow_source_links: true` 之前，discovery 會忽略它。

## 被拒絕的連結

指令會以非零狀態結束，且不會建立任何東西：

```text
$ skillshare link ~/.claude
✗ cannot link /home/me/.claude: target overlaps sync target /home/me/.claude/skills
```

| 原因 | 成因 |
|--------|-------|
| `target is the source or a parent of it` | 跟進它會繞回 source 本身 |
| `target is inside the source` | 其中的 skills 已經在 source 裡 |
| `target overlaps sync target <path>` | 資料夾就是、包含或位於某個啟用中的 skills target 內 |
| `target is a link chain that could not be resolved` | 該路徑是一個無法解析目標的連結 |
| `target is missing` / `target is not readable` / `target is unavailable: <error>` | 資料夾不存在或無法讀取 |
| `target is not a directory` | 該路徑是檔案 |
| `<name> already exists` | source 中已有同名項目 |
| `"<name>" is not a first-level name` | `--name` 含有路徑分隔符號，或是 `.` / `..` |

## 手動建立的連結

`skillshare link` 建立的項目與你自己建立的連結（`ln -s`，或 Windows 上的 `mklink /J`）相同，discovery 對兩者一視同仁。指令只是在建立之前先做上述檢查。

## 另請參閱

- [unlink](./unlink.md) — 移除連結
- [設定 — `follow_source_links`](../targets/configuration.md#follow_source_links) — discovery 行為、安全防護與透過連結寫入
- [doctor](./doctor.md) — 回報每個 source 連結的狀態
