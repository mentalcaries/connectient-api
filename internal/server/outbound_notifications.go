package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const (
	resendEndpoint = "https://api.resend.com/emails"
	twilioEndpoint = "https://api.twilio.com"
)

type outboundNotificationProvider struct {
	queries *db.Queries
	client  *http.Client
	env     func(string) string
}

type resendMessage struct {
	From    string `json:"from"`
	To      string `json:"to"`
	ReplyTo string `json:"reply_to,omitempty"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

func newOutboundNotificationProvider(queries *db.Queries) *outboundNotificationProvider {
	return &outboundNotificationProvider{
		queries: queries, client: &http.Client{Timeout: 10 * time.Second}, env: os.Getenv,
	}
}

func (p *outboundNotificationProvider) SendRegistrationEmail(ctx context.Context, notification RegistrationNotification) (RegistrationDeliveryResult, error) {
	if result, stop := p.channelSafety("email"); stop {
		return result, nil
	}
	if notification.PatientEmail == nil {
		return RegistrationDeliveryUnavailable, nil
	}
	practice, err := p.queries.GetNotificationPractice(ctx, notification.PracticeID)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	name := html.EscapeString(notification.PatientName)
	practiceName := html.EscapeString(practice.Name)
	link := html.EscapeString(notification.Link)
	practiceEmail, practicePhone := pointerString(practice.Email), pointerString(practice.Phone)
	message := resendMessage{
		From: fmt.Sprintf("%s <%s>", practice.Name, p.fromEmail()), To: *notification.PatientEmail,
		ReplyTo: practiceEmail, Subject: "Complete your registration with " + practice.Name,
		HTML: fmt.Sprintf(`<p>Hi %s,</p><p>%s has invited you to complete your patient registration form before your visit. This only takes a few minutes and will save time when you arrive.</p><p><a href="%s">Complete Registration</a></p><p>This link will expire in 7 days.</p><p>%s &bull; %s &bull; %s</p>`, name, practiceName, link, practiceName, html.EscapeString(practicePhone), html.EscapeString(practiceEmail)),
		Text: fmt.Sprintf("Hi %s,\n\n%s has invited you to complete your patient registration form before your visit. This only takes a few minutes and will save time when you arrive.\n\nComplete your registration here: %s\n\nThis link will expire in 7 days.\n\n%s • %s • %s\n\nYou can reply to this email or contact us at %s", notification.PatientName, practice.Name, notification.Link, practice.Name, practicePhone, practiceEmail, practiceEmail),
	}
	result, err := p.sendResend(ctx, message)
	if result == RegistrationDeliverySent {
		p.logDelivery(ctx, notification.PracticeID, "email", "registration_form")
	}
	return result, err
}

func (p *outboundNotificationProvider) SendRegistrationWhatsApp(ctx context.Context, notification RegistrationNotification) (RegistrationDeliveryResult, error) {
	if result, stop := p.channelSafety("whatsapp"); stop {
		return result, nil
	}
	if notification.PatientPhone == nil {
		return RegistrationDeliveryUnavailable, nil
	}
	practice, err := p.queries.GetNotificationPractice(ctx, notification.PracticeID)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	template := p.env("TWILIO_WHATSAPP_REGISTRATION_TEMPLATE")
	if template == "" {
		template = p.env("TWILIO_WHATSAPP_REGISTRATION_TEMPLATE_NO_CTA")
	}
	result, err := p.sendTwilio(ctx, *notification.PatientPhone, template, map[string]string{
		"1": firstName(notification.PatientName), "2": practice.Name, "3": registrationToken(notification.Link),
	})
	if result == RegistrationDeliverySent {
		p.logDelivery(ctx, notification.PracticeID, "whatsapp", "registration_form")
	}
	return result, err
}

func (p *outboundNotificationProvider) SendTeamInvite(ctx context.Context, notification TeamInviteNotification) (RegistrationDeliveryResult, error) {
	if result, stop := p.channelSafety("email"); stop {
		return result, nil
	}
	domain := strings.TrimSpace(p.env("EMAIL_SEND_FROM_DOMAIN"))
	if domain == "" {
		domain = "updates.connectient.app"
	}
	roleDisplay := "a Staff member"
	if notification.Role == "admin" {
		roleDisplay = "an Admin"
	}
	message := resendMessage{
		From: fmt.Sprintf("%s via Connectient <invites@%s>", notification.InviterName, domain),
		To:   notification.Email, Subject: "You've been invited to join " + notification.PracticeName + " on Connectient",
		HTML: fmt.Sprintf(`<p>Hi %s,</p><p>%s has invited you to join %s on Connectient as %s.</p><p><a href="%s">Accept Invitation</a></p><p>This link expires in 7 days. If you did not expect this invitation, you can ignore this email.</p>`,
			html.EscapeString(notification.FirstName), html.EscapeString(notification.InviterName),
			html.EscapeString(notification.PracticeName), html.EscapeString(roleDisplay), html.EscapeString(notification.Link)),
		Text: fmt.Sprintf("Hi %s,\n\n%s has invited you to join %s on Connectient as %s.\n\nClick the link below to accept your invitation. This link expires in 7 days.\n\n%s\n\nIf you did not expect this invitation, you can ignore this email.\n\nThe Connectient Team\nPlease do not reply to this email.", notification.FirstName,
			notification.InviterName, notification.PracticeName, roleDisplay, notification.Link),
	}
	return p.sendResend(ctx, message)
}

func (p *outboundNotificationProvider) SendAppointmentEmail(ctx context.Context, notification AppointmentNotification) (RegistrationDeliveryResult, error) {
	if result, stop := p.channelSafety("email"); stop {
		return result, nil
	}
	appointment, err := p.appointmentContext(ctx, notification)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	if strings.TrimSpace(notification.Email) == "" {
		return RegistrationDeliveryUnavailable, nil
	}
	details := formatAppointmentTime(notification.ScheduledDate, notification.ScheduledTime)
	providerName := strings.TrimSpace(pointerString(appointment.ProviderFirstName) + " " + pointerString(appointment.ProviderLastName))
	extraHTML, extraText := "", ""
	if providerName != "" {
		extraHTML += "<p>Provider: " + html.EscapeString(providerName) + "</p>"
		extraText += "\nProvider: " + providerName
	}
	if appointment.LocationName != nil {
		extraHTML += "<p>Location: " + html.EscapeString(*appointment.LocationName) + "</p>"
		extraText += "\nLocation: " + *appointment.LocationName
		if appointment.LocationAddress != nil {
			extraHTML += "<p>Address: " + html.EscapeString(*appointment.LocationAddress) + "</p>"
			extraText += "\nAddress: " + *appointment.LocationAddress
		}
	}
	message := resendMessage{
		From: fmt.Sprintf("%s via Connectient <%s>", appointment.PracticeName, p.fromEmail()),
		To:   notification.Email, ReplyTo: pointerString(appointment.PracticeEmail),
		Subject: "Your appointment with " + appointment.PracticeName + " is confirmed",
		HTML: fmt.Sprintf(`<p>Hi %s,</p><p>Your appointment at %s has been confirmed. Cancellations must be made at least 24 hours in advance.</p><p>Appointment Type: %s</p><p>Date and time: %s</p>%s`,
			html.EscapeString(notification.FirstName), html.EscapeString(appointment.PracticeName), html.EscapeString(notification.AppointmentType), html.EscapeString(details), extraHTML),
		Text: fmt.Sprintf("Hi %s,\n\nYour appointment at %s has been confirmed. Cancellations must be made at least 24 hours in advance.\n\nFull Name: %s %s\nAppointment Type: %s\nDate and time: %s%s\n\n%s • %s • %s\n\nYou can reply to this email or contact us at %s", notification.FirstName, appointment.PracticeName, notification.FirstName, notification.LastName, notification.AppointmentType, details, extraText, appointment.PracticeName, pointerString(appointment.PracticePhone), pointerString(appointment.PracticeEmail), pointerString(appointment.PracticeEmail)),
	}
	result, err := p.sendResend(ctx, message)
	if result == RegistrationDeliverySent {
		p.logDelivery(ctx, notification.PracticeID, "email", "appointment_confirmed")
	}
	return result, err
}

func (p *outboundNotificationProvider) SendAppointmentWhatsApp(ctx context.Context, notification AppointmentNotification) (RegistrationDeliveryResult, error) {
	if result, stop := p.channelSafety("whatsapp"); stop {
		return result, nil
	}
	appointment, err := p.appointmentContext(ctx, notification)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	result, err := p.sendTwilio(ctx, notification.MobilePhone, p.env("TWILIO_WHATSAPP_CONFIRM_TEMPLATE"), map[string]string{
		"1": notification.FirstName, "2": appointment.PracticeName,
		"3": formatAppointmentTime(notification.ScheduledDate, notification.ScheduledTime),
		"4": pointerString(appointment.PracticePhone),
	})
	if result == RegistrationDeliverySent {
		p.logDelivery(ctx, notification.PracticeID, "whatsapp", "appointment_confirmed")
	}
	return result, err
}

func (p *outboundNotificationProvider) NotifyStaffAppointmentRequest(ctx context.Context, notification PublicAppointmentRequestNotification) error {
	var failures []error
	if _, stop := p.channelSafety("email"); !stop && notification.PracticeEmail != nil {
		baseURL := strings.TrimRight(p.env("FRONTEND_BASE_URL"), "/")
		if baseURL == "" {
			baseURL = "https://connectient.app"
		}
		message := resendMessage{
			From: "Connectient <" + p.fromEmail() + ">", To: *notification.PracticeEmail,
			Subject: "New appointment request",
			HTML:    fmt.Sprintf(`<p>Hi there,</p><p>A new appointment has been requested via your Connectient booking page.</p><p>Log in to your dashboard to review and confirm it.</p><p><a href="%s/login">View Dashboard</a></p>`, html.EscapeString(baseURL)),
			Text:    "Hi there,\n\nA new appointment has been requested via your Connectient booking page.\n\nLog in to your dashboard to review and confirm it.\n\n" + baseURL + "/login\n\n© Connectient. All rights reserved.",
		}
		if result, err := p.sendResend(ctx, message); err != nil || result == RegistrationDeliveryFailed {
			failures = append(failures, err)
		} else if result == RegistrationDeliverySent {
			p.logDelivery(ctx, notification.PracticeID, "email", "new_appointment_request")
		}
	}
	if _, stop := p.channelSafety("whatsapp"); !stop {
		practiceID := notification.PracticeID
		phones, err := p.queries.ListEligibleNotificationStaffPhones(ctx, &practiceID)
		if err != nil {
			failures = append(failures, err)
		} else {
			for _, phone := range phones {
				if phone == nil {
					continue
				}
				result, sendErr := p.sendTwilio(ctx, *phone, p.env("TWILIO_WHATSAPP_APPT_TEMPLATE"), map[string]string{"1": notification.PracticeName})
				if sendErr != nil || result == RegistrationDeliveryFailed {
					failures = append(failures, sendErr)
				} else if result == RegistrationDeliverySent {
					p.logDelivery(ctx, notification.PracticeID, "whatsapp", "new_appointment_request")
				}
			}
		}
	}
	return errors.Join(failures...)
}

func (p *outboundNotificationProvider) channelSafety(channel string) (RegistrationDeliveryResult, bool) {
	if p.env("E2E_TEST_MODE") == "true" {
		return RegistrationDeliverySimulated, true
	}
	if p.env("OUTBOUND_MESSAGES_DISABLED") == "true" ||
		(channel == "email" && p.env("SKIP_EMAIL") == "true") ||
		(channel == "whatsapp" && p.env("SKIP_WHATSAPP") == "true") {
		return RegistrationDeliveryUnavailable, true
	}
	return "", false
}

func (p *outboundNotificationProvider) sendResend(ctx context.Context, message resendMessage) (RegistrationDeliveryResult, error) {
	apiKey := strings.TrimSpace(p.env("RESEND_API_KEY"))
	if apiKey == "" || strings.TrimSpace(message.To) == "" {
		return RegistrationDeliveryUnavailable, nil
	}
	body, err := json.Marshal(message)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, resendEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return RegistrationDeliveryFailed, fmt.Errorf("resend returned %s", response.Status)
	}
	return RegistrationDeliverySent, nil
}

func (p *outboundNotificationProvider) sendTwilio(ctx context.Context, to, contentSID string, variables map[string]string) (RegistrationDeliveryResult, error) {
	accountSID, token := strings.TrimSpace(p.env("TWILIO_ACCOUNT_SID")), strings.TrimSpace(p.env("TWILIO_AUTH_TOKEN"))
	from := strings.TrimSpace(p.env("TWILIO_WHATSAPP_NUMBER"))
	if accountSID == "" || token == "" || from == "" || strings.TrimSpace(to) == "" || strings.TrimSpace(contentSID) == "" {
		return RegistrationDeliveryUnavailable, nil
	}
	for _, value := range variables {
		if strings.TrimSpace(value) == "" {
			return RegistrationDeliveryUnavailable, nil
		}
	}
	encodedVariables, err := json.Marshal(variables)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	form := url.Values{
		"From":             {"whatsapp:" + from},
		"To":               {"whatsapp:" + strings.TrimSpace(to)},
		"ContentSid":       {contentSID},
		"ContentVariables": {string(encodedVariables)},
	}
	endpoint := twilioEndpoint + "/2010-04-01/Accounts/" + url.PathEscape(accountSID) + "/Messages.json"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	request.SetBasicAuth(accountSID, token)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := p.client.Do(request)
	if err != nil {
		return RegistrationDeliveryFailed, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return RegistrationDeliveryFailed, fmt.Errorf("twilio returned %s", response.Status)
	}
	return RegistrationDeliverySent, nil
}

func (p *outboundNotificationProvider) appointmentContext(ctx context.Context, notification AppointmentNotification) (db.GetNotificationAppointmentContextRow, error) {
	return p.queries.GetNotificationAppointmentContext(ctx, db.GetNotificationAppointmentContextParams{
		AppointmentID: notification.AppointmentID, PracticeID: notification.PracticeID,
	})
}

func (p *outboundNotificationProvider) logDelivery(ctx context.Context, practiceID uuid.UUID, channel, notificationType string) {
	_ = p.queries.CreateNotificationLog(ctx, db.CreateNotificationLogParams{
		PracticeID: practiceID, Channel: channel, NotificationType: notificationType,
	})
}

func (p *outboundNotificationProvider) fromEmail() string {
	if value := strings.TrimSpace(p.env("FROM_EMAIL")); value != "" {
		return value
	}
	return "noreply@connectient.app"
}

func firstName(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func registrationToken(link string) string {
	parsed, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return path.Base(strings.TrimRight(parsed.Path, "/"))
}

func formatAppointmentTime(date, clock string) string {
	if date == "" || clock == "" {
		return ""
	}
	parsed, err := time.Parse("2006-01-02 15:04:05", date+" "+clock)
	if err != nil {
		parsed, err = time.Parse("2006-01-02 15:04", date+" "+clock)
	}
	if err != nil {
		return date + " " + clock
	}
	return parsed.Format("Monday, January 2, 2006 at 3:04 PM")
}
