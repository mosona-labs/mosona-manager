package alerttasks

import (
	"fmt"
	"log"
	"strings"

	"mosona-manager/internal/config"
	"mosona-manager/internal/email"

	"github.com/nicholas-fedor/shoutrrr"
)

var (
	sendAlertEmail    = email.Send
	sendAlertShoutrrr = shoutrrr.Send
)

type notificationDelivery struct {
	attempted bool
	delivered bool
}

func (a *alertInstance) notifyAll(serverId int64, item, message string) notificationDelivery {
	var delivery notificationDelivery
	if len(a.notifications) == 0 {
		return delivery
	}

	serverName, ok := (*a.serverMap)[serverId]
	if !ok {
		serverName = fmt.Sprintf("Server %d", serverId)
	}

	uri := fmt.Sprintf("%s/%d/monitor", config.ReadDynamicConf().Domain, serverId)
	emailContent, err := email.GetNotificationTemplate(
		serverName,
		item,
		message,
		uri,
	)
	if err != nil {
		log.Println("failed to get email template:", err)
	}

	for _, n := range a.notifications {
		switch n.Module {
		case "email":
			delivery.attempted = true
			if emailContent != "" {
				if err := sendAlertEmail(n.Target, fmt.Sprintf("%s: %s - Alert Notification", serverName, item), emailContent); err != nil {
					log.Println("failed to send alert email:", err)
				} else {
					delivery.delivered = true
				}
			}
		case "shoutrrr":
			delivery.attempted = true
			target := n.Target
			if strings.Contains(target, "telegram://") && !strings.Contains(target, "parsemode=") {
				if strings.Contains(target, "?") {
					target += "&parsemode=HTML"
				} else {
					target += "?parsemode=HTML"
				}
			}
			if err := sendAlertShoutrrr(target, fmt.Sprintf("<b>%s – %s</b>\n%s\n\n%s", serverName, item, message, uri)); err != nil {
				log.Println("failed to send alert shoutrrr notification:", err)
			} else {
				delivery.delivered = true
			}
		}
	}

	return delivery
}
