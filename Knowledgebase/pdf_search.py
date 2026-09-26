from pypdf import PdfReader

PDF_PATH = "SCADA_Troubleshooting_Manual.pdf"


def search_alarm(alarm_id):
    reader = PdfReader(PDF_PATH)

    full_text = ""

    for page in reader.pages:
        text = page.extract_text()
        if text:
            full_text += text + "\n"

    # Find the alarm
    search_text = f"Alarm ID: {alarm_id}"

    start = full_text.find(search_text)

    if start == -1:
        return "Alarm not found in the PDF."

    # Find the next alarm section
    next_alarm = full_text.find("Alarm ID:", start + len(search_text))

    if next_alarm == -1:
        result = full_text[start:]
    else:
        result = full_text[start:next_alarm]

    return result


# Test
alarm = "PRESS_LUBE_OIL_LOW"

solution = search_alarm(alarm)

print("\n==============================")
print("TROUBLESHOOTING RESULT")
print("==============================\n")
print(solution)