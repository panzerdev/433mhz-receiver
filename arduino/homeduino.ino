#include <RFControl.h>

void rfcontrol_loop();

void setup() {
	Serial.begin(115200);
	Serial.print("ready\r\n");
	RFControl::startReceiving(0);
}

void loop() {
	rfcontrol_loop();
}

void rfcontrol_loop() {
    if(RFControl::hasData()) {
      unsigned int *timings;
      unsigned int timings_size;
      RFControl::getRaw(&timings, &timings_size);
      unsigned int buckets[8];
      unsigned int pulse_length_divider = RFControl::getPulseLengthDivider();
      RFControl::compressTimings(buckets, timings, timings_size);
      Serial.print("RF receive ");
      for(unsigned int i=0; i < 8; i++) {
        unsigned long bucket = buckets[i] * pulse_length_divider;
        Serial.print(bucket);
        Serial.write(' ');
      }
      for(unsigned int i=0; i < timings_size; i++) {
        Serial.write('0' + timings[i]);
      }
      Serial.print("\r\n");
      RFControl::continueReceiving();
    }
}
