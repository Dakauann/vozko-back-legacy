package conversation

func (o *SendOutcome) RecordedStatus() DeliveryStatus {
	if o == nil || o.DeliveryStatus == DeliveryStatusNone {
		return DeliveryStatusSent
	}
	return o.DeliveryStatus
}
