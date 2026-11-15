import { create } from '@bufbuild/protobuf'
import { previewClient } from '../../../api/clients'
import {
  PRPreviewSchema,
  UpsertPreviewRequestSchema,
  ListPreviewsRequestSchema,
  type PRPreview,
} from '../../../gen/v1/preview_pb'

export async function listPreviews(projectId = ''): Promise<PRPreview[]> {
  const response = await previewClient.listPreviews(
    create(ListPreviewsRequestSchema, { projectId }),
  )
  return response.previews
}

export async function upsertPreview(preview: {
  id?: string
  projectId: string
  repo: string
  prNumber: number
  status?: string
  previewUrl?: string
}): Promise<PRPreview> {
  const response = await previewClient.upsertPreview(
    create(UpsertPreviewRequestSchema, {
      preview: create(PRPreviewSchema, {
        id: preview.id ?? '',
        projectId: preview.projectId,
        repo: preview.repo,
        prNumber: preview.prNumber,
        status: preview.status ?? 'pending',
        previewUrl: preview.previewUrl ?? '',
        githubCommentId: 0n,
        createdAt: 0n,
      }),
    }),
  )
  if (!response.preview) throw new Error('Preview was not returned')
  return response.preview
}

/** No DestroyPreview RPC — mark row destroyed via Upsert (matches webhook semantics). */
export async function destroyPreview(item: PRPreview): Promise<PRPreview> {
  return upsertPreview({
    id: item.id,
    projectId: item.projectId,
    repo: item.repo,
    prNumber: item.prNumber,
    status: 'destroyed',
    previewUrl: item.previewUrl,
  })
}
