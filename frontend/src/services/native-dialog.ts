import { Dialogs } from '@wailsio/runtime'

import { translate } from '../locales/runtime'

export interface NativeFileDialogOptions {
  title: string
  message?: string
  directory?: string
  buttonText?: string
  filters?: Dialogs.FileFilter[]
}

/**
 * 使用统一 Wails 原生对话框选择单个文件。
 * @param options - 标题、初始目录和文件过滤器
 * @returns 用户选择的文件路径，取消时为 null
 */
export async function selectFile(options: NativeFileDialogOptions): Promise<string | null> {
  const path = await Dialogs.OpenFile({
    ...toOpenOptions(options),
    CanChooseFiles: true,
    CanChooseDirectories: false,
    AllowsMultipleSelection: false,
  })
  return path || null
}

/**
 * 使用统一 Wails 原生对话框选择目录。
 * @param options - 标题、提示和初始目录
 * @returns 用户选择的目录路径，取消时为 null
 */
export async function selectDirectory(options: NativeFileDialogOptions): Promise<string | null> {
  const path = await Dialogs.OpenFile({
    ...toOpenOptions(options),
    CanChooseFiles: false,
    CanChooseDirectories: true,
    CanCreateDirectories: true,
    AllowsMultipleSelection: false,
  })
  return path || null
}

/**
 * 使用统一 Wails 原生对话框选择保存路径。
 * @param options - 保存文件名、目录与过滤器
 * @returns 用户选择的保存路径，取消时为 null
 */
export async function selectSavePath(
  options: NativeFileDialogOptions & { filename?: string },
): Promise<string | null> {
  const dialogOptions: Dialogs.SaveFileDialogOptions = {
    ...toOpenOptions(options),
    CanCreateDirectories: true,
  }
  if (options.filename) dialogOptions.Filename = options.filename
  const path = await Dialogs.SaveFile(dialogOptions)
  return path || null
}

/**
 * 显示需要操作系统级确认的原生问题对话框。
 * @param options - 标题、正文与按钮标签
 * @returns 用户是否选择确认按钮
 */
export async function nativeConfirm(
  options: Dialogs.MessageDialogOptions & { confirmLabel?: string; cancelLabel?: string },
): Promise<boolean> {
  const confirmLabel = options.confirmLabel ?? translate('common.confirm')
  const cancelLabel = options.cancelLabel ?? translate('common.cancel')
  const selected = await Dialogs.Question({
    ...options,
    Buttons: [
      { Label: cancelLabel, IsCancel: true },
      { Label: confirmLabel, IsDefault: true },
    ],
  })
  return selected === confirmLabel
}

/**
 * 把统一的文件对话框参数转换成 Wails 打开对话框参数。
 * @param options - 统一的文件对话框参数
 * @returns Wails 打开文件对话框参数
 */
function toOpenOptions(options: NativeFileDialogOptions): Dialogs.OpenFileDialogOptions {
  const result: Dialogs.OpenFileDialogOptions = {
    Title: options.title,
  }
  if (options.message !== undefined) result.Message = options.message
  if (options.directory !== undefined) result.Directory = options.directory
  if (options.buttonText !== undefined) result.ButtonText = options.buttonText
  if (options.filters !== undefined) result.Filters = options.filters
  return result
}
