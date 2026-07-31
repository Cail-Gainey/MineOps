#!/usr/bin/env python3
"""根据 src/locales/messages/*.ts 重新生成 src/locales/resources.ts 聚合文件。"""
import os
import re

MESSAGES_DIR = 'src/locales/messages'
OUT = 'src/locales/resources.ts'

modules = []
seen = {}
for name in sorted(os.listdir(MESSAGES_DIR)):
    if not name.endswith('.ts'):
        continue
    text = open(os.path.join(MESSAGES_DIR, name), encoding='utf-8').read()
    match = re.search(r'export const (\w+) = defineMessages', text)
    if not match:
        raise SystemExit(f'no defineMessages export in {name}')
    # 键在聚合时按模块顺序覆盖，重复键会被静默吞掉，这里直接拒绝。
    for key in re.findall(r"^\s{4}'([^']+)':", text.split("'en-US': {")[0], re.M):
        if key in seen:
            raise SystemExit(f'duplicate message key {key} in {name} and {seen[key]}')
        seen[key] = name
    modules.append((match.group(1), name[:-3]))

modules.sort(key=lambda item: item[0])
imports = '\n'.join(f"import {{ {const} }} from './messages/{file}'" for const, file in modules)
zh = '\n'.join(f"    ...{const}['zh-CN']," for const, _ in modules)
en = '\n'.join(f"    ...{const}['en-US']," for const, _ in modules)

open(OUT, 'w', encoding='utf-8').write(f"""{imports}

export type {{ LocaleName }} from './define'

/**
 * 全量界面文案表。各语言模块通过 defineMessages 在编译期保证键集合一致，
 * 因此这里只需按模块聚合，无需再做运行时校验。
 *
 * 本文件由 scripts/generate-resources.py 依据 src/locales/messages 目录生成。
 */
export const resources = {{
  'zh-CN': {{
{zh}
  }},
  'en-US': {{
{en}
  }},
}}

/** 全量文案键，供 t() 与需要持有键名的模块使用。 */
export type ResourceKey = keyof (typeof resources)['zh-CN']
""")
print(f'generated {OUT} with {len(modules)} modules')
