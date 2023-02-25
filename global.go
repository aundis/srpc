package srpc

var globalClinet *Client = nil

func SetClinet(client *Client) {
	globalClinet = client
}

func GetClinet() *Client {
	if globalClinet == nil {
		panic("global client not set")
	}
	return globalClinet
}
