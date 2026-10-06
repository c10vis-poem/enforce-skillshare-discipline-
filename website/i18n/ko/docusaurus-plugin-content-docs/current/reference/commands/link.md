---
sidebar_position: 3
---

# link

폴더(예: 직접 관리하는 skills checkout)를 skills source 바로 아래에 링크합니다. [`follow_source_links`](../targets/configuration.md#follow_source_links)를 켜면 skillshare가 그 안의 skill을 나열하고 sync합니다.

```bash
skillshare link ~/code/dev-skills                  # _dev-skills로 링크
skillshare link ~/code/dev-skills --enable         # 링크하고 따라가기 켜기
skillshare link ~/code/dev-skills --name _mine     # 링크 이름 지정
skillshare link ../team --name _team -p            # project source에 링크
skillshare link -- -checkout                       # -로 시작하는 경로 링크
```

## 언제 사용하나요

- skills checkout을 복사본으로 설치하지 않고 원래 위치에서 계속 편집할 때
- [외장 드라이브](../targets/configuration.md#skills-on-an-external-drive)에 있는 skill을 사용할 때
- Windows에서 junction과 symlink 중 무엇을 쓸지 직접 고르지 않고 링크를 만들 때

```text
$ skillshare link ~/code/dev-skills --enable
✓ Linked _dev-skills → ~/code/dev-skills · symlink
✓ Set follow_source_links: true ~/.config/skillshare/config.yaml

Next
  skillshare sync  link its skills into your targets
```

## 동작 방식

1. 링크 이름의 기본값은 `_`와 폴더 이름입니다(`_dev-skills`). source 바로 아래의 단일 이름이어야 합니다.
2. discovery가 따라가는 링크를 검사하는 것과 같은 방식으로 폴더를 검사합니다. 이름이 이미 있거나, 폴더가 source 자체 또는 그 상위이거나, source 안에 있거나, sync target과 겹치거나, 없거나, 읽을 수 없거나, 디렉터리가 아니면 링크를 거부합니다.
3. 링크를 만듭니다. macOS와 Linux에서는 절대 경로 symlink입니다. Windows에서는 junction을 만들며 개발자 모드나 관리자 권한이 필요 없습니다. junction을 만들지 못했을 때만 디렉터리 symlink를 시도합니다.
4. 폴더에 `.git` 항목이 없으면 경고가 표시됩니다. 링크는 동작하지만 `skillshare update`로 pull할 수는 없습니다.
5. `follow_source_links`가 꺼져 있으면 `--enable`을 주지 않는 한 config를 바꾸지 않습니다. 주지 않으면 `skillshare doctor`와 같은 안내를 출력합니다. `--enable`로 config를 저장하지 못하면 새 링크를 다시 제거하고 명령이 실패하므로, 따라가지 않는 링크가 남지 않습니다. 대시보드의 **Link folder** 체크박스도 같습니다.

## 옵션

| Flag | Description |
|------|-------------|
| `--name <name>` | source 안의 링크 이름(기본값: `_<폴더 이름>`) |
| `--enable` | `follow_source_links: true`도 설정 |
| `--project, -p` | 현재 디렉터리의 project 레벨 config 사용 |
| `--global, -g` | global config 사용(`~/.config/skillshare`) |
| `--` | 옵션 끝. `-`로 시작하는 경로 허용 |
| `--help, -h` | 도움말 표시 |

## `--enable` 없이 실행할 때

```text
$ skillshare link ~/code/dev-skills
✓ Linked _dev-skills → ~/code/dev-skills · symlink
! target is not a git checkout; skillshare update cannot pull it
  _dev-skills: not followed by discovery; its contents are invisible to skillshare. Set follow_source_links: true to follow it
```

링크는 만들어졌지만, config에 `follow_source_links: true`가 설정될 때까지 discovery는 이를 무시합니다.

## 거부되는 링크

명령은 0이 아닌 코드로 종료하며 아무것도 만들지 않습니다:

```text
$ skillshare link ~/.claude
✗ cannot link /home/me/.claude: target overlaps sync target /home/me/.claude/skills
```

| 이유 | 원인 |
|--------|-------|
| `target is the source or a parent of it` | 따라가면 source 자신으로 되돌아옵니다 |
| `target is inside the source` | 그 skill은 이미 source에 있습니다 |
| `target overlaps sync target <path>` | 폴더가 활성 skills target 자체이거나, 이를 포함하거나, 그 안에 있습니다 |
| `target is a link chain that could not be resolved` | 경로가 대상을 해석할 수 없는 링크입니다 |
| `target is missing` / `target is not readable` / `target is unavailable: <error>` | 폴더가 없거나 읽을 수 없습니다 |
| `target is not a directory` | 경로가 파일입니다 |
| `<name> already exists` | source에 같은 이름의 항목이 이미 있습니다 |
| `"<name>" is not a first-level name` | `--name`에 경로 구분자가 있거나 `.` / `..`입니다 |

## 수동 링크

`skillshare link`가 만드는 항목은 직접 만든 링크(`ln -s`, Windows에서는 `mklink /J`)와 같으며, discovery는 둘을 똑같이 다룹니다. 이 명령은 만들기 전에 위의 검사를 추가할 뿐입니다.

## 참고

- [unlink](./unlink.md) — 링크 제거
- [Configuration — `follow_source_links`](../targets/configuration.md#follow_source_links) — discovery 동작, 안전 장치, 링크를 통한 쓰기
- [doctor](./doctor.md) — 각 source 링크의 상태 보고
