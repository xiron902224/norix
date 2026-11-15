import { create } from '@bufbuild/protobuf'
import { wasmClient } from '../../../api/clients'
import {
  CreateWasmFunctionRequestSchema,
  ListWasmFunctionsRequestSchema,
  type WasmFunction,
} from '../../../gen/v1/wasm_pb'

export async function listWasmFunctions(projectId = ''): Promise<WasmFunction[]> {
  const response = await wasmClient.listWasmFunctions(
    create(ListWasmFunctionsRequestSchema, { projectId }),
  )
  return response.functions
}

export async function createWasmFunction(input: {
  projectId: string
  name: string
  imageRef: string
  fuelLimit?: bigint
  memoryLimitMb?: number
}): Promise<WasmFunction> {
  const response = await wasmClient.createWasmFunction(
    create(CreateWasmFunctionRequestSchema, {
      projectId: input.projectId,
      name: input.name,
      imageRef: input.imageRef,
      fuelLimit: input.fuelLimit ?? 1_000_000_000n,
      memoryLimitMb: input.memoryLimitMb ?? 64,
    }),
  )
  if (!response.function) throw new Error('Wasm function was not returned')
  return response.function
}
