import type { components } from "@semlia/sdk-typescript";
import { apiClient } from "./apiClient";

export type EmbeddingStatus = components["schemas"]["EmbeddingIndexStatus"];
export type EmbeddingIndex = components["schemas"]["EmbeddingIndexVersion"];
export type EmbeddingSearchResult = components["schemas"]["EmbeddingSearchResult"];
export interface EmbeddingApi {
 status(workspaceId:string,signal?:AbortSignal):Promise<EmbeddingStatus>;
 start(workspaceId:string,key:string):Promise<EmbeddingIndex>;
 cancel(workspaceId:string,indexId:string):Promise<void>;
 search(workspaceId:string,query:string,signal?:AbortSignal):Promise<EmbeddingSearchResult>;
}
export const embeddingApi:EmbeddingApi={
 async status(workspaceId,signal){const {data,error}=await apiClient.GET("/api/v1/workspaces/{workspaceId}/embedding-index",{params:{path:{workspaceId}},signal});if(error||!data)throw new Error("无法读取向量索引状态");return data},
 async start(workspaceId,key){const {data,error}=await apiClient.POST("/api/v1/workspaces/{workspaceId}/embedding-index",{params:{path:{workspaceId},header:{"Idempotency-Key":key}}});if(error||!data)throw new Error("无法启动重建，请检查服务配置与权限");return data},
 async cancel(workspaceId,indexId){const {error}=await apiClient.POST("/api/v1/workspaces/{workspaceId}/embedding-index/{indexId}/cancel",{params:{path:{workspaceId,indexId}}});if(error)throw new Error("取消重建失败")},
 async search(workspaceId,q,signal){const {data,error}=await apiClient.GET("/api/v1/workspaces/{workspaceId}/embedding-search",{params:{path:{workspaceId},query:{q}},signal});if(error||!data)throw new Error("检索失败");return data},
};
