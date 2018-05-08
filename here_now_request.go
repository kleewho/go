package pubnub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	//"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/pubnub/go/pnerr"
)

var hereNowPath = "/v2/presence/sub_key/%s/channel/%s"
var globalHereNowPath = "/v2/presence/sub_key/%s"

var emptyHereNowResponse *HereNowResponse

type hereNowBuilder struct {
	opts *hereNowOpts
}

func newHereNowBuilder(pubnub *PubNub) *hereNowBuilder {
	builder := hereNowBuilder{
		opts: &hereNowOpts{
			pubnub: pubnub,
		},
	}

	return &builder
}

func newHereNowBuilderWithContext(pubnub *PubNub,
	context Context) *hereNowBuilder {
	builder := hereNowBuilder{
		opts: &hereNowOpts{
			pubnub: pubnub,
			ctx:    context,
		},
	}

	return &builder
}

func (b *hereNowBuilder) Channels(ch []string) *hereNowBuilder {
	b.opts.Channels = ch

	return b
}

func (b *hereNowBuilder) ChannelGroups(cg []string) *hereNowBuilder {
	b.opts.ChannelGroups = cg

	return b
}

func (b *hereNowBuilder) IncludeState(state bool) *hereNowBuilder {
	b.opts.IncludeState = state
	b.opts.SetIncludeState = true

	return b
}

func (b *hereNowBuilder) IncludeUuids(uuid bool) *hereNowBuilder {
	b.opts.IncludeUuids = uuid
	b.opts.SetIncludeUuids = true

	return b
}

func (b *hereNowBuilder) Execute() (*HereNowResponse, StatusResponse, error) {
	rawJson, status, err := executeRequest(b.opts)
	if err != nil {
		return emptyHereNowResponse, status, err
	}

	return newHereNowResponse(rawJson, b.opts.Channels, status)
}

type hereNowOpts struct {
	pubnub *PubNub

	Channels      []string
	ChannelGroups []string

	IncludeUuids bool
	IncludeState bool

	SetIncludeState bool
	SetIncludeUuids bool

	Transport http.RoundTripper

	ctx Context
}

func (o *hereNowOpts) config() Config {
	return *o.pubnub.Config
}

func (o *hereNowOpts) client() *http.Client {
	return o.pubnub.GetClient()
}

func (o *hereNowOpts) context() Context {
	return o.ctx
}

func (o *hereNowOpts) validate() error {
	if o.config().SubscribeKey == "" {
		return newValidationError(o, StrMissingSubKey)
	}

	return nil
}

func (o *hereNowOpts) buildPath() (string, error) {
	if len(o.Channels) == 0 && len(o.ChannelGroups) == 0 {
		return fmt.Sprintf(globalHereNowPath,
			o.pubnub.Config.SubscribeKey), nil
	}

	if len(o.Channels) == 0 {
		return fmt.Sprintf(hereNowPath,
			o.pubnub.Config.SubscribeKey,
			","), nil
	}

	return fmt.Sprintf(hereNowPath,
		o.pubnub.Config.SubscribeKey,
		strings.Join(o.Channels, ",")), nil
}

func (o *hereNowOpts) buildQuery() (*url.Values, error) {
	q := defaultQuery(o.pubnub.Config.Uuid, o.pubnub.telemetryManager)

	if len(o.ChannelGroups) > 0 {
		q.Set("channel-group", strings.Join(o.ChannelGroups, ","))
	}

	if o.SetIncludeState && o.IncludeState {
		q.Set("state", "1")
	} else if o.SetIncludeState && !o.IncludeState {
		q.Set("state", "0")
	}

	if o.SetIncludeUuids && !o.IncludeUuids {
		q.Set("disable-uuids", "1")
	} else if o.SetIncludeUuids && o.IncludeUuids {
		q.Set("disable-uuids", "0")
	}

	return q, nil
}

func (o *hereNowOpts) buildBody() ([]byte, error) {
	return []byte{}, nil
}

func (o *hereNowOpts) httpMethod() string {
	return "GET"
}

func (o *hereNowOpts) isAuthRequired() bool {
	return true
}

func (o *hereNowOpts) requestTimeout() int {
	return o.pubnub.Config.NonSubscribeRequestTimeout
}

func (o *hereNowOpts) connectTimeout() int {
	return o.pubnub.Config.ConnectTimeout
}

func (o *hereNowOpts) operationType() OperationType {
	return PNHereNowOperation
}

func (o *hereNowOpts) telemetryManager() *TelemetryManager {
	return o.pubnub.telemetryManager
}

type HereNowResponse struct {
	TotalChannels  int
	TotalOccupancy int

	Channels []HereNowChannelData
}

type HereNowChannelData struct {
	ChannelName string

	Occupancy int

	Occupants []HereNowOccupantsData
}

type HereNowOccupantsData struct {
	Uuid string

	State map[string]interface{}
}

func newHereNowResponse(jsonBytes []byte, channelNames []string,
	status StatusResponse) (*HereNowResponse, StatusResponse, error) {
	resp := &HereNowResponse{}

	var value interface{}

	err := json.Unmarshal(jsonBytes, &value)
	if err != nil {
		e := pnerr.NewResponseParsingError("Error unmarshalling response",
			ioutil.NopCloser(bytes.NewBufferString(string(jsonBytes))), err)

		return emptyHereNowResponse, status, e
	}

	if parsedValue, ok := value.(map[string]interface{}); ok {
		// multiple
		if payload, ok := parsedValue["payload"]; ok {
			channels := []HereNowChannelData{}

			if parsedPayload, ok := payload.(map[string]interface{}); ok {
				if val, ok := parsedPayload["channels"].(map[string]interface{}); ok {
					if len(val) > 0 {
						for channelName, rawData := range val {
							channels = append(channels, parseChannelData(channelName, rawData))
						}

						if totalCh, ok := parsedPayload["total_channels"].(float64); ok {
							resp.TotalChannels = int(totalCh)
						}

						if totalOcc, ok := parsedPayload["total_occupancy"].(float64); ok {
							resp.TotalOccupancy = int(totalOcc)
						}

						resp.Channels = channels

						return resp, status, nil
					} else if len(val) == 1 {
						resp.TotalChannels = 1

						if totalOcc, ok := parsedPayload["total_occupancy"].(float64); ok {
							resp.TotalOccupancy = int(totalOcc)
						}

						resp.Channels = append(resp.Channels, HereNowChannelData{
							channelNames[0], 1, []HereNowOccupantsData{},
						})

						return resp, status, nil
					} else {
						if totalCh, ok := parsedValue["total_channels"].(float64); ok {
							resp.TotalChannels = int(totalCh)
						}

						if totalOcc, ok := parsedValue["total_occupancy"].(float64); ok {
							resp.TotalOccupancy = int(totalOcc)
						}

						return resp, status, nil
					}
				}
			}
			// empty
		} else if occupancy, ok := parsedValue["occupancy"].(int); ok && occupancy == 0 {
			if totalCh, ok := parsedValue["total_channels"].(int); ok {
				resp.TotalChannels = totalCh
			}

			if totalOcc, ok := parsedValue["total_occupancy"].(int); ok {
				resp.TotalOccupancy = totalOcc
			}

			resp.Channels = append(resp.Channels, HereNowChannelData{
				channelNames[0], 0, []HereNowOccupantsData{},
			})

			return resp, status, nil
			// single
		} else if _, ok := parsedValue["uuids"]; ok {
			if uuids, ok := parsedValue["uuids"].([]interface{}); ok {
				occupants := []HereNowOccupantsData{}
				for _, user := range uuids {
					if u, ok := user.(string); ok {
						empty := make(map[string]interface{})
						occupants = append(occupants, HereNowOccupantsData{u, empty})
					} else if parsedUser, ok := user.(map[string]interface{}); ok {
						state := make(map[string]interface{})

						if u, ok := parsedUser["state"].(map[string]interface{}); ok {
							state = u
						}

						var uuid string

						if val, ok := parsedUser["uuid"].(string); ok {
							uuid = val
						}

						occupants = append(occupants, HereNowOccupantsData{
							uuid, state,
						})
					}
				}

				resp.TotalChannels = 1

				var occup int

				if occupancy, ok := parsedValue["occupancy"].(float64); ok {
					occup = int(occupancy)
					resp.TotalOccupancy = int(occupancy)
				}

				resp.Channels = append(resp.Channels, HereNowChannelData{
					channelNames[0],
					occup,
					occupants,
				})
			}

			return resp, status, nil
		} else {
			resp.TotalChannels = 1

			var occup int
			if occupancy, ok := parsedValue["occupancy"].(int); ok {
				occup = occupancy
				resp.TotalOccupancy = occupancy
			}

			resp.Channels = append(resp.Channels, HereNowChannelData{
				channelNames[0],
				occup,
				[]HereNowOccupantsData{},
			})

			return resp, status, nil
		}
	}

	return resp, status, nil
}

func parseChannelData(channelName string, rawData interface{}) HereNowChannelData {
	chData := HereNowChannelData{}
	occupants := []HereNowOccupantsData{}

	if parsedRawData, ok := rawData.(map[string]interface{}); ok {
		if uuids, ok := parsedRawData["uuids"]; ok {
			if parsedUuids, ok := uuids.([]interface{}); ok {
				for _, user := range parsedUuids {
					if u, ok := user.(map[string]interface{}); ok {
						if len(u) > 0 {
							if _, ok := u["state"]; ok {
								occData := HereNowOccupantsData{}

								if uuid, ok := u["uuid"].(string); ok {
									occData.Uuid = uuid
								}

								if state, ok := u["state"].(map[string]interface{}); ok {
									//log.Println(u)
									occData.State = state
								}

								occupants = append(occupants, occData)
							} else {
								occData := HereNowOccupantsData{}

								if uuid, ok := u["uuid"].(string); ok {
									occData.Uuid = uuid
								}

								occupants = append(occupants, occData)
							}
						}
					} else {
						empty := make(map[string]interface{})

						if u, ok := user.(string); ok {
							occupants = append(occupants, HereNowOccupantsData{u, empty})
						}
					}
				}
			}
		}
		chData.ChannelName = channelName
		chData.Occupants = occupants

		if occupancy, ok := parsedRawData["occupancy"].(float64); ok {
			chData.Occupancy = int(occupancy)
		}
	}

	return chData
}
