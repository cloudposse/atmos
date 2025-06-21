package dtos

type UnlockStackRequest struct {
	Key string `json:"key"`
}

type UnlockStackResponse struct {
	AtmosApiResponse
	Data struct{} `json:"data"`
}
