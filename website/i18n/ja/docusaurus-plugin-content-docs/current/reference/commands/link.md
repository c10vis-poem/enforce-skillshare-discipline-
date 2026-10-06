---
sidebar_position: 3
---

# link

フォルダ（自分の Skill のチェックアウトなど）を Skill の source の直下にリンクします。[`follow_source_links`](../targets/configuration.md#follow_source_links) を有効にすると、skillshare はその中の Skill を一覧表示し、sync します。

```bash
skillshare link ~/code/dev-skills                  # _dev-skills としてリンク
skillshare link ~/code/dev-skills --enable         # リンクしてたどる設定を有効化
skillshare link ~/code/dev-skills --name _mine     # リンク名を指定
skillshare link ../team --name _team -p            # project の source にリンク
skillshare link -- -checkout                       # - で始まるパスをリンク
```

## 使うタイミング

- Skill のチェックアウトをコピーとしてインストールせず、今ある場所で編集し続ける
- [外部ドライブ](../targets/configuration.md#skills-on-an-external-drive)上の Skill を使う
- Windows で、ジャンクションと symlink のどちらにするかを自分で選ばずにリンクを作成する

```text
$ skillshare link ~/code/dev-skills --enable
✓ Linked _dev-skills → ~/code/dev-skills · symlink
✓ Set follow_source_links: true ~/.config/skillshare/config.yaml

Next
  skillshare sync  link its skills into your targets
```

## 実行内容

1. リンク名のデフォルトは `_` とフォルダ名です（`_dev-skills`）。source の直下にある単一の名前でなければなりません。
2. discovery がたどるリンクを確認するのと同じ方法でフォルダを確認します。名前が既に存在する場合、またはフォルダが source 自体やその親である、source の内部にある、sync target と重なる、存在しない、読み取れない、ディレクトリではない場合、リンクは拒否されます。
3. リンクを作成します。macOS と Linux では絶対パスの symlink です。Windows ではジャンクションを作成し、開発者モードも管理者権限も不要です。ジャンクションの作成に失敗した場合にのみディレクトリ symlink を試します。
4. フォルダに `.git` エントリがない場合は警告が表示されます。リンクは機能しますが、`skillshare update` で pull することはできません。
5. `follow_source_links` が無効の場合、`--enable` を付けない限り config は変更されません。付けない場合、`skillshare doctor` と同じヒントが表示されます。`--enable` で config を保存できない場合、作成したリンクは削除されてコマンドは失敗するため、たどられないリンクは残りません。ダッシュボードの **Link folder** のチェックボックスも同じです。

## オプション

| フラグ | 説明 |
|------|-------------|
| `--name <name>` | source 内のリンク名（デフォルト: `_<フォルダ名>`） |
| `--enable` | `follow_source_links: true` も設定 |
| `--project, -p` | カレントディレクトリの project レベルの config を使用 |
| `--global, -g` | global config を使用（`~/.config/skillshare`） |
| `--` | オプションの終わり。`-` で始まるパスを指定できる |
| `--help, -h` | ヘルプを表示 |

## `--enable` を付けない場合

```text
$ skillshare link ~/code/dev-skills
✓ Linked _dev-skills → ~/code/dev-skills · symlink
! target is not a git checkout; skillshare update cannot pull it
  _dev-skills: not followed by discovery; its contents are invisible to skillshare. Set follow_source_links: true to follow it
```

リンクは作成されていますが、config で `follow_source_links: true` が設定されるまで discovery はそれを無視します。

## 拒否されるリンク

コマンドは非ゼロで終了し、何も作成しません。

```text
$ skillshare link ~/.claude
✗ cannot link /home/me/.claude: target overlaps sync target /home/me/.claude/skills
```

| 理由 | 原因 |
|--------|-------|
| `target is the source or a parent of it` | たどると source 自体に戻ってしまう |
| `target is inside the source` | その Skill は既に source にある |
| `target overlaps sync target <path>` | フォルダが有効な skills target そのものである、それを含む、またはその中にある |
| `target is a link chain that could not be resolved` | パスがリンクで、そのリンク先を解決できない |
| `target is missing` / `target is not readable` / `target is unavailable: <error>` | フォルダが存在しないか、読み取れない |
| `target is not a directory` | パスがファイルである |
| `<name> already exists` | source に同じ名前のものが既にある |
| `"<name>" is not a first-level name` | `--name` にパス区切り文字が含まれている、または `.` / `..` である |

## 手動のリンク

`skillshare link` が作成するエントリは、自分で作成したリンク（`ln -s`、Windows では `mklink /J`）と同じもので、discovery は両者を同じように扱います。このコマンドは、作成前に上記のチェックを加えるだけです。

## 関連項目

- [unlink](./unlink.md) — リンクを削除
- [Configuration — `follow_source_links`](../targets/configuration.md#follow_source_links) — discovery の動作、安全ガード、リンク経由の書き込み
- [doctor](./doctor.md) — 各 source リンクの状態を報告
